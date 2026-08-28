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

package extractor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// --- Mock infrastructure ---

type mockClient struct {
	mu       sync.Mutex
	handlers []func(msgs []conversation.Message) []llm.StreamEvent
	callIdx  int
}

func (m *mockClient) SetSystemPrompt(string) {}

func (m *mockClient) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan llm.StreamEvent, <-chan error) {
	ch := make(chan llm.StreamEvent, 64)
	errCh := make(chan error, 1)
	msgs := conv.GetMessages()
	m.mu.Lock()
	idx := m.callIdx
	m.callIdx++
	m.mu.Unlock()
	go func() {
		defer close(ch)
		defer close(errCh)
		if idx >= len(m.handlers) {
			ch <- llm.StreamEnd{StopReason: "end_turn"}
			return
		}
		for _, ev := range m.handlers[idx](msgs) {
			ch <- ev
		}
	}()
	return ch, errCh
}

type mockTool struct {
	name string
	cat  tools.ToolCategory
	exec func(args map[string]any) tools.ToolResult
}

func (t *mockTool) Name() string                 { return t.name }
func (t *mockTool) Description() string          { return "mock " + t.name }
func (t *mockTool) Category() tools.ToolCategory { return t.cat }
func (t *mockTool) Schema() map[string]any {
	return map[string]any{
		"name":         t.name,
		"description":  "mock",
		"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func (t *mockTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	if t.exec != nil {
		return t.exec(args)
	}
	return tools.ToolResult{Output: "ok"}
}

// --- Unit tests for pure helpers ---

func TestGetWrittenFilePath(t *testing.T) {
	cases := []struct {
		name string
		tu   conversation.ToolUseBlock
		want string
	}{
		{"WriteFile with path", conversation.ToolUseBlock{ToolName: "WriteFile", Arguments: map[string]any{"file_path": "/a/b.md"}}, "/a/b.md"},
		{"EditFile with path", conversation.ToolUseBlock{ToolName: "EditFile", Arguments: map[string]any{"file_path": "/a/c.md"}}, "/a/c.md"},
		{"ReadFile returns empty", conversation.ToolUseBlock{ToolName: "ReadFile", Arguments: map[string]any{"file_path": "/a/b.md"}}, ""},
		{"no file_path", conversation.ToolUseBlock{ToolName: "WriteFile", Arguments: map[string]any{}}, ""},
		{"non-string file_path", conversation.ToolUseBlock{ToolName: "WriteFile", Arguments: map[string]any{"file_path": 42}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getWrittenFilePath(tc.tu); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractWrittenPathsDedupes(t *testing.T) {
	msgs := []conversation.Message{
		{Role: "assistant", ToolUses: []conversation.ToolUseBlock{
			{ToolName: "WriteFile", Arguments: map[string]any{"file_path": "/m/a.md"}},
			{ToolName: "EditFile", Arguments: map[string]any{"file_path": "/m/MEMORY.md"}},
		}},
		{Role: "user"},
		{Role: "assistant", ToolUses: []conversation.ToolUseBlock{
			{ToolName: "WriteFile", Arguments: map[string]any{"file_path": "/m/a.md"}}, // dup
			{ToolName: "ReadFile", Arguments: map[string]any{"file_path": "/m/a.md"}},  // not Write/Edit
			{ToolName: "WriteFile", Arguments: map[string]any{"file_path": "/m/b.md"}},
		}},
	}
	got := extractWrittenPaths(msgs)
	want := []string{"/m/a.md", "/m/MEMORY.md", "/m/b.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCountModelVisibleMessagesSince(t *testing.T) {
	msgs := []conversation.Message{
		{Role: "user"},      // 0
		{Role: "assistant"}, // 1
		{Role: "user"},      // 2
		{Role: "tool"},      // 3 — not model-visible
		{Role: "assistant"}, // 4
	}
	if got := countModelVisibleMessagesSince(msgs, 0); got != 4 {
		t.Errorf("from 0: got %d, want 4", got)
	}
	if got := countModelVisibleMessagesSince(msgs, 2); got != 2 {
		t.Errorf("from 2: got %d, want 2 (user@2 + assistant@4)", got)
	}
	if got := countModelVisibleMessagesSince(msgs, 5); got != 0 {
		t.Errorf("cursor at end should return 0 new messages: got %d", got)
	}
	if got := countModelVisibleMessagesSince(msgs, 10); got != 4 {
		t.Errorf("out-of-range cursor (>len) should fall back to total: got %d, want 4", got)
	}
	if got := countModelVisibleMessagesSince(msgs, -1); got != 4 {
		t.Errorf("negative cursor should fall back to total: got %d, want 4", got)
	}
}

func TestHasMemoryWritesSince(t *testing.T) {
	tmp := t.TempDir()
	memDir := memory.GetAutoMemPath(tmp) // <tmp>/.swiftx/memory/
	memFile := memDir + "user_role.md"
	outsideFile := filepath.Join(tmp, "other.md")

	msgs := []conversation.Message{
		{Role: "user"},
		{Role: "assistant", ToolUses: []conversation.ToolUseBlock{
			{ToolName: "WriteFile", Arguments: map[string]any{"file_path": outsideFile}},
		}},
		{Role: "user"},
		{Role: "assistant", ToolUses: []conversation.ToolUseBlock{
			{ToolName: "WriteFile", Arguments: map[string]any{"file_path": memFile}},
		}},
	}

	if hasMemoryWritesSince(msgs, 0, tmp) != true {
		t.Error("should detect memory write at idx 3")
	}
	// Skip past the memory write: cursor at 4 → nothing left
	if hasMemoryWritesSince(msgs, 4, tmp) != false {
		t.Error("cursor past all writes should return false")
	}
	// Cursor at 2: still has the memory write at 3
	if hasMemoryWritesSince(msgs, 2, tmp) != true {
		t.Error("cursor before memory write should return true")
	}
	// Only outside-memory writes
	subset := msgs[:2]
	if hasMemoryWritesSince(subset, 0, tmp) != false {
		t.Error("outside-memory writes alone should not trigger skip")
	}
}

// --- Integration: full extraction round trip ---

func TestExtractorEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	memDir := memory.GetAutoMemPath(tmp)
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Parent conversation with one user/assistant exchange — the extractor
	// will see this as "newMessageCount" worth of content to consider.
	parent := conversation.NewManager()
	parent.AddUserMessage("Remember I'm a Go engineer")
	parent.AddAssistantMessage("Sure, I'll remember.")

	// Forked agent script: write user_role.md, end turn.
	writePath := filepath.Join(memDir, "user_role.md")
	client := &mockClient{handlers: []func([]conversation.Message) []llm.StreamEvent{
		func(_ []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: "Saving."},
				llm.ToolCallStart{ToolName: "WriteFile", ToolID: "w1"},
				llm.ToolCallComplete{
					ToolID:   "w1",
					ToolName: "WriteFile",
					Arguments: map[string]any{
						"file_path": writePath,
						"content":   "---\nname: user-role\ntype: user\n---\n\nGo engineer.\n",
					},
				},
				llm.StreamEnd{StopReason: "tool_use"},
			}
		},
		func(_ []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: "Done."},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
	}}

	reg := tools.NewRegistry()
	reg.Register(&mockTool{
		name: "WriteFile", cat: tools.CategoryWrite,
		exec: func(args map[string]any) tools.ToolResult {
			fp, _ := args["file_path"].(string)
			content, _ := args["content"].(string)
			if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
				return tools.ToolResult{Output: err.Error(), IsError: true}
			}
			return tools.ToolResult{Output: "wrote " + fp}
		},
	})

	var savedMsg string
	var savedMu sync.Mutex
	deps := Deps{
		MemoryDir:    memDir,
		ProjectRoot:  tmp,
		Client:       client,
		ToolRegistry: reg,
		Protocol:     "anthropic",
		Conversation: parent,
		AppendSystem: func(s string) {
			savedMu.Lock()
			savedMsg = s
			savedMu.Unlock()
		},
	}

	e := InitExtractMemories(deps)
	if err := e.Execute(context.Background()); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Verify file landed on disk
	if _, err := os.Stat(writePath); err != nil {
		t.Fatalf("expected user_role.md to exist after extraction: %v", err)
	}

	savedMu.Lock()
	got := savedMsg
	savedMu.Unlock()
	if !strings.Contains(got, "Memory saved: user_role.md") {
		t.Errorf("AppendSystem should announce saved memory, got %q", got)
	}

	// Cursor should have advanced past all parent messages
	if e.lastMemoryMessageIdx != len(parent.GetMessages()) {
		t.Errorf("cursor not advanced: got %d, want %d",
			e.lastMemoryMessageIdx, len(parent.GetMessages()))
	}
}

func TestExtractorSkipsWhenMainAgentWroteMemory(t *testing.T) {
	tmp := t.TempDir()
	memDir := memory.GetAutoMemPath(tmp)
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	parent := conversation.NewManager()
	parent.AddUserMessage("remember X")
	parent.AddAssistantMessageWithTools("Sure.", []conversation.ToolUseBlock{
		{
			ToolUseID: "w1",
			ToolName:  "WriteFile",
			Arguments: map[string]any{"file_path": memDir + "x.md"},
		},
	})

	called := false
	client := &mockClient{handlers: []func([]conversation.Message) []llm.StreamEvent{
		func(_ []conversation.Message) []llm.StreamEvent {
			called = true
			return []llm.StreamEvent{llm.StreamEnd{StopReason: "end_turn"}}
		},
	}}

	deps := Deps{
		MemoryDir:    memDir,
		ProjectRoot:  tmp,
		Client:       client,
		ToolRegistry: tools.NewRegistry(),
		Protocol:     "anthropic",
		Conversation: parent,
	}
	e := InitExtractMemories(deps)
	if err := e.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if called {
		t.Error("subagent must not run when main agent already wrote memory")
	}
	if e.lastMemoryMessageIdx != len(parent.GetMessages()) {
		t.Errorf("cursor must advance past direct-write range: got %d", e.lastMemoryMessageIdx)
	}
}

func TestExtractorDrainIdleReturnsImmediately(t *testing.T) {
	e := InitExtractMemories(Deps{})
	start := time.Now()
	_ = e.Drain(5000)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("Drain on idle extractor should be instant, took %s", elapsed)
	}
}

func TestExtractorBuildExtractorConversationAppendsPrompt(t *testing.T) {
	parent := conversation.NewManager()
	parent.AddUserMessage("hi")
	parent.AddAssistantMessage("hello")

	forked := buildExtractorConversation(parent, "EXTRACTION_PROMPT")
	msgs := forked.GetMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (parent 2 + prompt), got %d", len(msgs))
	}
	if msgs[2].Role != "user" || !strings.Contains(msgs[2].Content, "EXTRACTION_PROMPT") {
		t.Errorf("last message should be user with prompt; got %+v", msgs[2])
	}
	// Forked is a new Manager — modifying it must not touch parent
	if len(parent.GetMessages()) != 2 {
		t.Error("parent conversation was mutated")
	}
}
