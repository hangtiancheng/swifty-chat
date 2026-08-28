// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Tests for tool-result budget wiring in the agent main loop.

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tool_result"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// These tests drive the full Agent main loop and verify the tool-result budget
// wiring at the point results enter history: single-result spilling, aggregate
// spilling, readback exemption, and that the content stored in the conversation
// history is already in its final form.

func findToolResultsMsg(t *testing.T, conv *conversation.Manager) conversation.Message {
	t.Helper()
	for _, m := range conv.GetMessages() {
		if len(m.ToolResults) > 0 {
			return m
		}
	}
	t.Fatal("no tool-results message in conversation")
	return conversation.Message{}
}

func TestIngestSpillsSingleOversizedResult(t *testing.T) {
	big := strings.Repeat("x", 60000)
	client := &mockClient{responses: [][]llm.StreamEvent{
		{
			llm.ToolCallStart{ToolName: "BigTool", ToolID: "t1"},
			llm.ToolCallComplete{ToolID: "t1", ToolName: "BigTool", Arguments: map[string]any{}},
			llm.StreamEnd{StopReason: "tool_use"},
		},
		{
			llm.TextDelta{Text: "done"},
			llm.StreamEnd{StopReason: "end_turn"},
		},
	}}
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "BigTool", result: big})
	ag := New(client, reg, "anthropic")
	ag.WorkDir = t.TempDir()
	conv := conversation.NewManager()

	_, events := runConversationRound(ag, conv, "go")

	// The content stored in history is a preview, not the raw original.
	msg := findToolResultsMsg(t, conv)
	got := msg.ToolResults[0].Content
	if !strings.HasPrefix(got, "<persisted-output>") {
		t.Fatalf("history content should be a preview, got %d chars: %q...", len(got), got[:40])
	}
	// The spill file saved the complete raw original.
	fi, err := os.Stat(filepath.Join(tool_result.SpillDir(ag.WorkDir, ""), "t1.txt"))
	if err != nil {
		t.Fatalf("spill file missing: %v", err)
	}
	if fi.Size() != 60000 {
		t.Fatalf("spill file size = %d, want 60000", fi.Size())
	}
	// The UI event carries the raw output.
	trs := getToolResults(events)
	if len(trs) != 1 || len(trs[0].Output) != 60000 {
		t.Fatalf("UI event should carry raw output, got %d results", len(trs))
	}
}

func TestIngestReadbackExempt(t *testing.T) {
	ag0WorkDir := t.TempDir()
	readbackPath := filepath.Join(tool_result.SpillDir(ag0WorkDir, ""), "toolu_old.txt")
	big := strings.Repeat("y", 60000)

	client := &mockClient{responses: [][]llm.StreamEvent{
		{
			llm.ToolCallStart{ToolName: "ReadFile", ToolID: "t_rb"},
			llm.ToolCallComplete{ToolID: "t_rb", ToolName: "ReadFile", Arguments: map[string]any{"file_path": readbackPath}},
			llm.StreamEnd{StopReason: "tool_use"},
		},
		{
			llm.TextDelta{Text: "done"},
			llm.StreamEnd{StopReason: "end_turn"},
		},
	}}
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "ReadFile", result: big})
	ag := New(client, reg, "anthropic")
	ag.WorkDir = ag0WorkDir
	conv := conversation.NewManager()

	runConversationRound(ag, conv, "read it back")

	// The readback result is exempt from spilling: the raw original enters history.
	msg := findToolResultsMsg(t, conv)
	if got := msg.ToolResults[0].Content; len(got) != 60000 {
		t.Fatalf("readback should stay raw, got %d chars", len(got))
	}
	// No new spill file was generated for the readback result.
	if _, err := os.Stat(filepath.Join(tool_result.SpillDir(ag0WorkDir, ""), "t_rb.txt")); !os.IsNotExist(err) {
		t.Fatal("readback result must not be spilled")
	}
}

func TestIngestAggregateSpillsLargest(t *testing.T) {
	// 5 parallel tools each ~45K; none exceeds the single-result limit, but the
	// combined 225K exceeds the aggregate threshold, so only the largest (t3) is spilled.
	sizes := map[string]int{"T1": 45000, "T2": 45000, "T3": 45001, "T4": 45000, "T5": 45000}
	var first []llm.StreamEvent
	reg := tools.NewRegistry()
	for _, name := range []string{"T1", "T2", "T3", "T4", "T5"} {
		id := "t" + strings.ToLower(name[1:])
		first = append(first,
			llm.ToolCallStart{ToolName: name, ToolID: id},
			llm.ToolCallComplete{ToolID: id, ToolName: name, Arguments: map[string]any{}},
		)
		reg.Register(&mockTool{name: name, result: strings.Repeat("z", sizes[name])})
	}
	first = append(first, llm.StreamEnd{StopReason: "tool_use"})

	client := &mockClient{responses: [][]llm.StreamEvent{
		first,
		{llm.TextDelta{Text: "done"}, llm.StreamEnd{StopReason: "end_turn"}},
	}}
	ag := New(client, reg, "anthropic")
	ag.WorkDir = t.TempDir()
	conv := conversation.NewManager()

	runConversationRound(ag, conv, "fan out")

	msg := findToolResultsMsg(t, conv)
	total := 0
	previews := 0
	var t3Content string
	for _, tr := range msg.ToolResults {
		total += len(tr.Content)
		if strings.HasPrefix(tr.Content, "<persisted-output>") {
			previews++
		}
		if tr.ToolUseID == "t3" {
			t3Content = tr.Content
		}
	}
	if total > 200000 {
		t.Fatalf("aggregate %d still over limit", total)
	}
	if previews != 1 {
		t.Fatalf("expected exactly 1 preview, got %d", previews)
	}
	if !strings.HasPrefix(t3Content, "<persisted-output>") {
		t.Fatal("largest result t3 should be the one spilled")
	}
}
