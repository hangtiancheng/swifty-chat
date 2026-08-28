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

package agent

import (
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

func recallCh(reminder string, paths ...string) <-chan RecallResult {
	ch := make(chan RecallResult, 1)
	ch <- RecallResult{Reminder: reminder, Paths: paths}
	return ch
}

// Turn with tool calls: the recall result is injected after tool results and marked as surfaced.
func TestMemoryRecallInjectedAfterTools(t *testing.T) {
	client := &mockClient{responses: [][]llm.StreamEvent{
		{
			llm.ToolCallStart{ToolName: "ReadFile", ToolID: "t1"},
			llm.ToolCallComplete{ToolID: "t1", ToolName: "ReadFile", Arguments: map[string]any{"file_path": "/tmp/x"}},
			llm.StreamEnd{StopReason: "tool_use"},
		},
		{llm.TextDelta{Text: "done"}, llm.StreamEnd{StopReason: "end_turn"}},
	}}
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "ReadFile", result: "ok"})
	ag := New(client, reg, "anthropic")
	ag.MemoryRecallCh = recallCh("## Memory: a.md", "/mem/a.md")
	conv := conversation.NewManager()
	runConversationRound(ag, conv, "read it")

	found := false
	for _, m := range conv.GetMessages() {
		if strings.Contains(m.Content, "## Memory: a.md") {
			found = true
		}
	}
	if !found {
		t.Fatal("recall reminder should be injected after tool results")
	}
	_, surfaced := ag.RecallHints()
	if _, ok := surfaced["/mem/a.md"]; !ok {
		t.Error("injected memory should be marked surfaced")
	}
}

// Turn without tool calls: the recall result is not consumed, so the corresponding
// memories must not be marked as surfaced and remain eligible for the next recall.
func TestMemoryRecallNotSurfacedWithoutTools(t *testing.T) {
	client := &mockClient{responses: [][]llm.StreamEvent{{
		llm.TextDelta{Text: "plain answer"},
		llm.StreamEnd{StopReason: "end_turn"},
	}}}
	ag := New(client, tools.NewRegistry(), "anthropic")
	ag.MemoryRecallCh = recallCh("## Memory: a.md", "/mem/a.md")
	conv := conversation.NewManager()
	runConversationRound(ag, conv, "hi")

	for _, m := range conv.GetMessages() {
		if strings.Contains(m.Content, "## Memory: a.md") {
			t.Fatal("recall reminder must not be injected when no tool ran")
		}
	}
	_, surfaced := ag.RecallHints()
	if len(surfaced) != 0 {
		t.Errorf("unconsumed recall must not be marked surfaced, got %v", surfaced)
	}
}
