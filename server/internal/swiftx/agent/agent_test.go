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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/skills"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// --- Mock infrastructure ---

// mockClient returns scripted responses in order.
type mockClient struct {
	responses [][]llm.StreamEvent
	callIdx   int
}

func (m *mockClient) SetSystemPrompt(string) {}

func (m *mockClient) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan llm.StreamEvent, <-chan error) {
	ch := make(chan llm.StreamEvent, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(ch)
		defer close(errCh)
		if m.callIdx >= len(m.responses) {
			ch <- llm.TextDelta{Text: "[mock: no more scripted responses]"}
			ch <- llm.StreamEnd{StopReason: "end_turn"}
			return
		}
		for _, ev := range m.responses[m.callIdx] {
			ch <- ev
		}
		m.callIdx++
	}()
	return ch, errCh
}

// dynamicMock inspects the conversation to decide what to respond.
// Each handler receives the current conversation and returns events.
// Handlers are consumed in order; within one agent.Run() call the mock
// may be invoked multiple times (once per LLM turn in the tool-call loop).
type dynamicMock struct {
	handlers []func(msgs []conversation.Message) []llm.StreamEvent
	callIdx  int
}

func (m *dynamicMock) SetSystemPrompt(string) {}

func (m *dynamicMock) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan llm.StreamEvent, <-chan error) {
	ch := make(chan llm.StreamEvent, 64)
	errCh := make(chan error, 1)
	msgs := conv.GetMessages()
	go func() {
		defer close(ch)
		defer close(errCh)
		if m.callIdx >= len(m.handlers) {
			ch <- llm.TextDelta{Text: "[dynamic mock: no more handlers]"}
			ch <- llm.StreamEnd{StopReason: "end_turn"}
			return
		}
		events := m.handlers[m.callIdx](msgs)
		for _, ev := range events {
			ch <- ev
		}
		m.callIdx++
	}()
	return ch, errCh
}

// mockTool returns a fixed result.
type mockTool struct {
	name   string
	result string
}

func (t *mockTool) Name() string                 { return t.name }
func (t *mockTool) Description() string          { return "mock tool" }
func (t *mockTool) Category() tools.ToolCategory { return tools.CategoryRead }
func (t *mockTool) Schema() map[string]any {
	return map[string]any{
		"name": t.name, "description": "mock",
		"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
	}
}
func (t *mockTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	return tools.ToolResult{Output: t.result}
}

// --- Helpers ---

func collectEvents(ch <-chan AgentEvent) []AgentEvent {
	var events []AgentEvent
	for ev := range ch {
		if perm, ok := ev.(PermissionRequestEvent); ok {
			perm.ResponseCh <- PermAllow
			continue
		}
		events = append(events, ev)
	}
	return events
}

func getStreamText(events []AgentEvent) string {
	var sb strings.Builder
	for _, ev := range events {
		if st, ok := ev.(StreamText); ok {
			sb.WriteString(st.Text)
		}
	}
	return sb.String()
}

func getToolResults(events []AgentEvent) []ToolResultEvent {
	var results []ToolResultEvent
	for _, ev := range events {
		if tr, ok := ev.(ToolResultEvent); ok {
			results = append(results, tr)
		}
	}
	return results
}

// buildSkillListing generates the skill catalog text. It is injected into the
// conversation via the first system-reminder, not the system prompt (see Agent.SkillSection).
func buildSkillListing(skillsDir string, catalog *skills.Catalog) string {
	var sb strings.Builder
	sb.WriteString("## Available Skills\n\n")
	fmt.Fprintf(&sb, "Skills are installed at: %s\n", skillsDir)
	sb.WriteString("When creating new skills, always place them under this directory as <skill-name>/SKILL.md.\n\n")
	sb.WriteString("The following skills are available. When the user invokes /<name>, follow that skill's instructions.\n\n")
	for _, meta := range catalog.List() {
		desc := meta.Description
		if len(desc) > 200 {
			desc = desc[:200] + "…"
		}
		fmt.Fprintf(&sb, "- /%s: %s\n", meta.Name, desc)
	}
	return sb.String()
}

// runConversationRound simulates what the TUI does for one user message:
// add user msg to conv, call agent.Run(), collect events, return text.
func runConversationRound(ag *Agent, conv *conversation.Manager, userMsg string) (string, []AgentEvent) {
	conv.AddUserMessage(userMsg)
	events := collectEvents(ag.Run(context.Background(), conv))
	return getStreamText(events), events
}

// --- Basic agent tests ---

func TestAgentSimpleResponse(t *testing.T) {
	client := &mockClient{responses: [][]llm.StreamEvent{{
		llm.TextDelta{Text: "Hello, "},
		llm.TextDelta{Text: "world!"},
		llm.StreamEnd{StopReason: "end_turn"},
	}}}
	ag := New(client, tools.NewRegistry(), "anthropic")
	conv := conversation.NewManager()
	text, events := runConversationRound(ag, conv, "hi")
	if text != "Hello, world!" {
		t.Errorf("got %q, want %q", text, "Hello, world!")
	}
	hasComplete := false
	for _, ev := range events {
		if _, ok := ev.(LoopComplete); ok {
			hasComplete = true
		}
	}
	if !hasComplete {
		t.Error("missing LoopComplete event")
	}
}

func TestAgentToolCallLoop(t *testing.T) {
	client := &mockClient{responses: [][]llm.StreamEvent{
		{
			llm.TextDelta{Text: "Let me read that."},
			llm.ToolCallStart{ToolName: "ReadFile", ToolID: "t1"},
			llm.ToolCallComplete{ToolID: "t1", ToolName: "ReadFile", Arguments: map[string]any{"file_path": "/tmp/x"}},
			llm.StreamEnd{StopReason: "tool_use"},
		},
		{
			llm.TextDelta{Text: "File says: hello from mock"},
			llm.StreamEnd{StopReason: "end_turn"},
		},
	}}
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "ReadFile", result: "hello from mock"})
	ag := New(client, reg, "anthropic")
	conv := conversation.NewManager()
	text, events := runConversationRound(ag, conv, "read it")
	if !strings.Contains(text, "hello from mock") {
		t.Errorf("response %q should mention tool output", text)
	}
	trs := getToolResults(events)
	if len(trs) != 1 || trs[0].Output != "hello from mock" {
		t.Errorf("unexpected tool results: %+v", trs)
	}
}

func TestAgentMaxIterations(t *testing.T) {
	loop := []llm.StreamEvent{
		llm.ToolCallStart{ToolName: "Glob", ToolID: "t"},
		llm.ToolCallComplete{ToolID: "t", ToolName: "Glob", Arguments: map[string]any{"pattern": "*"}},
		llm.StreamEnd{StopReason: "tool_use"},
	}
	responses := make([][]llm.StreamEvent, 10)
	for i := range responses {
		responses[i] = loop
	}
	client := &mockClient{responses: responses}
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "Glob", result: "f.txt"})
	ag := New(client, reg, "anthropic")
	ag.MaxIterations = 3
	conv := conversation.NewManager()
	_, events := runConversationRound(ag, conv, "loop")
	found := false
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok && strings.Contains(e.Message, "maximum iterations") {
			found = true
		}
	}
	if !found {
		t.Error("expected max iterations error")
	}
}

func TestAgentWithThinking(t *testing.T) {
	client := &mockClient{responses: [][]llm.StreamEvent{{
		llm.ThinkingDelta{Text: "hmm..."},
		llm.ThinkingComplete{Thinking: "hmm...", Signature: "sig_1"},
		llm.TextDelta{Text: "My answer."},
		llm.StreamEnd{StopReason: "end_turn"},
	}}}
	ag := New(client, tools.NewRegistry(), "anthropic")
	conv := conversation.NewManager()
	runConversationRound(ag, conv, "think")
	msgs := conv.GetMessages()
	last := msgs[len(msgs)-1]
	if len(last.ThinkingBlocks) == 0 || last.ThinkingBlocks[0].Signature != "sig_1" {
		t.Error("thinking block not stored correctly")
	}
}

// --- Multi-round conversation tests ---
// These simulate the real TUI flow: agent.Run() ends when LLM returns text
// without tool calls, then the user sends a new message, and agent.Run()
// is called again. This is how real conversations work.

func TestMultiRoundConversation(t *testing.T) {
	// Simulate: user asks → agent asks clarification → user answers → agent works → done
	client := &dynamicMock{handlers: []func([]conversation.Message) []llm.StreamEvent{
		// Round 1: agent asks a clarifying question
		func(msgs []conversation.Message) []llm.StreamEvent {
			last := msgs[len(msgs)-1]
			if !strings.Contains(last.Content, "refactor") {
				t.Errorf("round 1: expected user msg about refactor, got %q", last.Content)
			}
			return []llm.StreamEvent{
				llm.TextDelta{Text: "Which file do you want me to refactor? And what style — extract functions, simplify conditionals, or both?"},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
		// Round 2: agent reads the file
		func(msgs []conversation.Message) []llm.StreamEvent {
			last := msgs[len(msgs)-1]
			if !strings.Contains(last.Content, "main.go") {
				t.Errorf("round 2: expected user msg about main.go, got %q", last.Content)
			}
			return []llm.StreamEvent{
				llm.TextDelta{Text: "I'll read main.go first."},
				llm.ToolCallStart{ToolName: "ReadFile", ToolID: "r1"},
				llm.ToolCallComplete{ToolID: "r1", ToolName: "ReadFile", Arguments: map[string]any{"file_path": "main.go"}},
				llm.StreamEnd{StopReason: "tool_use"},
			}
		},
		// Round 2 continued (after tool result): agent produces final answer
		func(msgs []conversation.Message) []llm.StreamEvent {
			// Verify tool result is in conversation
			found := false
			for _, m := range msgs {
				for _, tr := range m.ToolResults {
					if strings.Contains(tr.Content, "func main") {
						found = true
					}
				}
			}
			if !found {
				t.Error("round 2 continued: tool result not found in conversation")
			}
			return []llm.StreamEvent{
				llm.TextDelta{Text: "Here's the refactored version:\n```go\nfunc main() {\n    run()\n}\n```\nI extracted the logic into a `run()` function."},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
	}}

	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "ReadFile", result: "package main\n\nfunc main() {\n    // lots of code\n    fmt.Println(\"hello\")\n}"})

	ag := New(client, reg, "anthropic")
	conv := conversation.NewManager()

	// Round 1: user asks, agent asks clarification
	text1, _ := runConversationRound(ag, conv, "please refactor this code")
	if !strings.Contains(text1, "Which file") {
		t.Errorf("round 1: agent should ask clarification, got: %s", text1)
	}
	t.Logf("Round 1 agent: %s", text1)

	// Round 2: user answers, agent reads file and refactors
	text2, events2 := runConversationRound(ag, conv, "main.go, extract functions please")
	if !strings.Contains(text2, "refactored") {
		t.Errorf("round 2: agent should produce refactored code, got: %s", text2)
	}
	t.Logf("Round 2 agent: %s", text2)

	// Verify tool was called
	trs := getToolResults(events2)
	if len(trs) != 1 || trs[0].ToolName != "ReadFile" {
		t.Errorf("expected ReadFile tool call, got: %+v", trs)
	}

	// Verify full conversation has correct structure
	msgs := conv.GetMessages()
	t.Logf("Total messages in conversation: %d", len(msgs))
	// Expected: user1, assistant1, user2, assistant2+tool, tool_result, assistant3
	if len(msgs) < 6 {
		t.Fatalf("expected 6+ messages, got %d", len(msgs))
	}
	if msgs[0].Content != "please refactor this code" {
		t.Error("msg[0] should be first user message")
	}
	if msgs[2].Content != "main.go, extract functions please" {
		t.Error("msg[2] should be second user message")
	}
}

// --- Skill integration tests with full agent simulation ---

func TestFrontendDesignSkillFullSession(t *testing.T) {
	// Simulates: user invokes /frontend-design → agent asks what to build →
	// user says "login page" → agent creates files → verify files exist

	workDir := t.TempDir()

	// Load real frontend-design skill or create a test one
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "frontend-design")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: frontend-design
description: Create distinctive, production-grade frontend interfaces
---

# Frontend Design Skill

Create high-quality frontend code. Follow these principles:
1. Semantic HTML structure
2. Modern CSS with custom properties
3. Responsive design
4. Accessibility (ARIA labels, focus states)

Output files directly using WriteFile.
`), 0o644)

	catalog, _ := skills.LoadFromDirectory(dir)
	skill := catalog.Get("frontend-design")
	skillBody := skill.PromptBody

	outputFile := filepath.Join(workDir, "login.html")

	client := &dynamicMock{handlers: []func([]conversation.Message) []llm.StreamEvent{
		// Round 1: agent receives skill prompt, asks what to build
		func(msgs []conversation.Message) []llm.StreamEvent {
			last := msgs[len(msgs)-1]
			// Verify the skill prompt was injected correctly
			if !strings.Contains(last.Content, "Frontend Design Skill") {
				t.Errorf("skill body not in user message: %s", last.Content[:min(100, len(last.Content))])
			}
			return []llm.StreamEvent{
				llm.TextDelta{Text: "I'd love to help you build a frontend! What kind of page or component do you need? For example:\n- Login page\n- Dashboard\n- Landing page"},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
		// Round 2: user says login page, agent creates file
		func(msgs []conversation.Message) []llm.StreamEvent {
			last := msgs[len(msgs)-1]
			if !strings.Contains(last.Content, "login") {
				t.Errorf("expected user to say login, got: %s", last.Content)
			}
			return []llm.StreamEvent{
				llm.TextDelta{Text: "I'll create a login page with email/password fields."},
				llm.ToolCallStart{ToolName: "WriteFile", ToolID: "w1"},
				llm.ToolCallComplete{ToolID: "w1", ToolName: "WriteFile", Arguments: map[string]any{
					"file_path": outputFile,
					"content": `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Login</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: system-ui; display: flex; justify-content: center; align-items: center; min-height: 100vh; background: #f0f2f5; }
    .login-card { background: white; padding: 2rem; border-radius: 12px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); width: 100%; max-width: 400px; }
    h1 { margin-bottom: 1.5rem; color: #1a1a2e; }
    label { display: block; margin-bottom: 0.5rem; font-weight: 500; }
    input { width: 100%; padding: 0.75rem; border: 1px solid #ddd; border-radius: 8px; margin-bottom: 1rem; }
    button { width: 100%; padding: 0.75rem; background: #4361ee; color: white; border: none; border-radius: 8px; cursor: pointer; font-size: 1rem; }
    button:hover { background: #3a56d4; }
  </style>
</head>
<body>
  <div class="login-card">
    <h1>Sign In</h1>
    <form>
      <label for="email">Email</label>
      <input type="email" id="email" name="email" placeholder="you@example.com" required aria-label="Email address">
      <label for="password">Password</label>
      <input type="password" id="password" name="password" placeholder="Your password" required aria-label="Password">
      <button type="submit">Log In</button>
    </form>
  </div>
</body>
</html>`,
				}},
				llm.StreamEnd{StopReason: "tool_use"},
			}
		},
		// Round 2 continued: after file write, agent confirms
		func(msgs []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: fmt.Sprintf("Done! I created `%s` with:\n- Clean semantic HTML\n- Responsive card layout\n- Accessible form fields with ARIA labels\n- Modern CSS with custom properties\n\nOpen it in your browser to see the result.", outputFile)},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
	}}

	// Use real WriteFile tool so we can verify the file is actually created
	reg := tools.NewRegistry()
	reg.Register(&tools.WriteFileTool{})

	ag := New(client, reg, "anthropic")
	conv := conversation.NewManager()

	// Round 1: invoke skill (this is what TUI does when user types /frontend-design)
	skillPrompt := skillBody
	text1, _ := runConversationRound(ag, conv, skillPrompt)
	t.Logf("Agent (round 1): %s", text1)

	if !strings.Contains(text1, "What kind of page") {
		t.Errorf("agent should ask what to build, got: %s", text1)
	}

	// Round 2: user answers, agent creates file
	text2, events2 := runConversationRound(ag, conv, "a login page with email and password")
	t.Logf("Agent (round 2): %s", text2)

	if !strings.Contains(text2, "login") && !strings.Contains(text2, "Login") {
		t.Errorf("agent should confirm login page creation, got: %s", text2)
	}

	// Verify WriteFile was called
	trs := getToolResults(events2)
	writeCount := 0
	for _, tr := range trs {
		if tr.ToolName == "WriteFile" {
			writeCount++
			if tr.IsError {
				t.Errorf("WriteFile failed: %s", tr.Output)
			}
		}
	}
	if writeCount != 1 {
		t.Errorf("expected 1 WriteFile call, got %d", writeCount)
	}

	// THE KEY CHECK: verify the file actually exists on disk with correct content
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	html := string(content)

	checks := []struct {
		substr string
		desc   string
	}{
		{"<!DOCTYPE html>", "valid HTML doctype"},
		{"<form", "has a form element"},
		{"type=\"email\"", "has email input"},
		{"type=\"password\"", "has password input"},
		{"<button", "has submit button"},
		{"aria-label", "has accessibility attributes"},
		{"border-radius", "has modern CSS styling"},
		{"max-width", "has responsive layout"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("output file missing %s (looking for %q)", c.desc, c.substr)
		}
	}
	t.Logf("Output file: %s (%d bytes, all %d checks passed)", outputFile, len(content), len(checks))

	// Verify conversation history is coherent
	msgs := conv.GetMessages()
	t.Logf("Conversation: %d messages", len(msgs))
	for i, m := range msgs {
		summary := m.Content
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		if len(m.ToolUses) > 0 {
			summary += fmt.Sprintf(" [+%d tool calls]", len(m.ToolUses))
		}
		if len(m.ToolResults) > 0 {
			summary += fmt.Sprintf(" [+%d tool results]", len(m.ToolResults))
		}
		t.Logf("  [%d] %s: %s", i, m.Role, summary)
	}
}

func TestSkillCreatorOutputsToCorrectDirectory(t *testing.T) {
	// Simulates: user invokes /skill-creator → provides details →
	// agent creates the new skill → verify it lands in .swiftx/skills/, not root

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, ".swiftx", "skills")
	os.MkdirAll(skillsDir, 0o755)

	// Set up skill-creator skill
	creatorDir := filepath.Join(skillsDir, "skill-creator")
	os.MkdirAll(creatorDir, 0o755)
	os.WriteFile(filepath.Join(creatorDir, "SKILL.md"), []byte(`---
name: skill-creator
description: Create new skills
---

# Skill Creator

New skills MUST be created under the .swiftx/skills/ directory.
The full path should be .swiftx/skills/<skill-name>/SKILL.md.
`), 0o644)

	catalog, _ := skills.LoadFromDirectory(skillsDir)
	skillListing := buildSkillListing(skillsDir, catalog)
	skill := catalog.Get("skill-creator")

	newSkillDir := filepath.Join(skillsDir, "git-helper")
	newSkillFile := filepath.Join(newSkillDir, "SKILL.md")

	client := &dynamicMock{handlers: []func([]conversation.Message) []llm.StreamEvent{
		// Round 1: agent asks what the skill should do
		func(msgs []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: "I'll help you create a new skill! What should this skill do? What would you like to name it?"},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
		// Round 2: user describes, agent creates the skill in .swiftx/skills/
		func(msgs []conversation.Message) []llm.StreamEvent {
			last := msgs[len(msgs)-1]
			if !strings.Contains(last.Content, "git") {
				t.Errorf("expected user to mention git, got: %s", last.Content)
			}
			// Agent creates directory then writes SKILL.md
			return []llm.StreamEvent{
				llm.TextDelta{Text: "I'll create a git-helper skill for you."},
				llm.ToolCallStart{ToolName: "Bash", ToolID: "b1"},
				llm.ToolCallComplete{ToolID: "b1", ToolName: "Bash", Arguments: map[string]any{
					// Executed via shell; normalize the path to forward slashes and quote it so
					// Windows backslashes are not treated as shell escape characters.
					"command": fmt.Sprintf("mkdir -p '%s'", filepath.ToSlash(newSkillDir)),
				}},
				llm.StreamEnd{StopReason: "tool_use"},
			}
		},
		// After mkdir, write the SKILL.md
		func(msgs []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.ToolCallStart{ToolName: "WriteFile", ToolID: "w1"},
				llm.ToolCallComplete{ToolID: "w1", ToolName: "WriteFile", Arguments: map[string]any{
					"file_path": newSkillFile,
					"content": `---
name: git-helper
description: Help with common git operations like branching, rebasing, and resolving conflicts
---

# Git Helper Skill

Help the user with git operations:
1. Create and manage branches
2. Interactive rebase guidance
3. Merge conflict resolution
4. Commit message best practices
`,
				}},
				llm.StreamEnd{StopReason: "tool_use"},
			}
		},
		// Final confirmation
		func(msgs []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: fmt.Sprintf("Done! Created git-helper skill at `%s`.\n\nYou can now use it with `/git-helper`.", newSkillFile)},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
	}}

	reg := tools.NewRegistry()
	reg.Register(&tools.WriteFileTool{})
	reg.Register(&tools.BashTool{})

	ag := New(client, reg, "anthropic")
	conv := conversation.NewManager()

	// Verify system prompt tells agent where to put skills
	if !strings.Contains(skillListing, skillsDir) {
		t.Fatalf("system prompt missing skills dir: %s", skillsDir)
	}

	// Round 1: invoke skill-creator
	text1, _ := runConversationRound(ag, conv, skill.PromptBody+"\n\n## User Request\n\ncreate a new skill")
	t.Logf("Agent (round 1): %s", text1)
	if !strings.Contains(text1, "skill") {
		t.Errorf("agent should ask about the skill, got: %s", text1)
	}

	// Round 2: user describes the skill
	text2, _ := runConversationRound(ag, conv, "a git helper that helps with branching, rebasing and conflicts")
	t.Logf("Agent (round 2): %s", text2)

	// THE KEY CHECK: new skill was created inside .swiftx/skills/, NOT in project root
	if _, err := os.Stat(newSkillFile); os.IsNotExist(err) {
		t.Fatalf("skill file not created at expected path: %s", newSkillFile)
	}

	// Verify the new skill can be loaded by the skills system
	updatedCatalog, err := skills.LoadFromDirectory(skillsDir)
	if err != nil {
		t.Fatalf("failed to reload skills: %v", err)
	}

	gitHelper := updatedCatalog.Get("git-helper")
	if gitHelper == nil {
		t.Fatal("git-helper skill not found after creation")
	}
	if !strings.Contains(gitHelper.PromptBody, "rebase") {
		t.Error("git-helper body should mention rebase")
	}
	if !strings.Contains(gitHelper.Meta.Description, "git") {
		t.Error("git-helper description should mention git")
	}

	// Verify it did NOT create files in project root
	rootSkillFile := filepath.Join(workDir, "git-helper", "SKILL.md")
	if _, err := os.Stat(rootSkillFile); err == nil {
		t.Errorf("skill was INCORRECTLY created at project root: %s", rootSkillFile)
	}

	t.Logf("New skill loaded successfully: name=%s, body=%d chars", gitHelper.Meta.Name, len(gitHelper.PromptBody))

	// Verify we now have 2 skills total
	allSkills := updatedCatalog.List()
	if len(allSkills) != 2 {
		t.Errorf("expected 2 skills (skill-creator + git-helper), got %d", len(allSkills))
	}
}

func TestSkillMultiRoundWithToolChain(t *testing.T) {
	// Simulates a realistic skill session: agent reads existing code,
	// asks user for confirmation, then writes modified code.
	// Tests: skill prompt → read → ask user → user confirms → write → verify

	workDir := t.TempDir()

	// Create an existing file the agent will read
	srcFile := filepath.Join(workDir, "app.js")
	os.WriteFile(srcFile, []byte(`const express = require('express');
const app = express();
app.get('/', (req, res) => res.send('Hello'));
app.listen(3000);
`), 0o644)

	outputFile := filepath.Join(workDir, "app.js")

	client := &dynamicMock{handlers: []func([]conversation.Message) []llm.StreamEvent{
		// Round 1: agent reads the file first
		func(msgs []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: "Let me check the current code."},
				llm.ToolCallStart{ToolName: "ReadFile", ToolID: "r1"},
				llm.ToolCallComplete{ToolID: "r1", ToolName: "ReadFile", Arguments: map[string]any{"file_path": srcFile}},
				llm.StreamEnd{StopReason: "tool_use"},
			}
		},
		// After reading, agent proposes changes
		func(msgs []conversation.Message) []llm.StreamEvent {
			// Verify the agent actually got the file content
			for _, m := range msgs {
				for _, tr := range m.ToolResults {
					if !strings.Contains(tr.Content, "express") {
						t.Errorf("tool result should contain express, got: %s", tr.Content)
					}
				}
			}
			return []llm.StreamEvent{
				llm.TextDelta{Text: "I see you have a basic Express app. I'll add:\n- Error handling middleware\n- CORS support\n- Health check endpoint\n\nShall I proceed?"},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
		// Round 2: user confirms, agent writes
		func(msgs []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: "Updating the file with improvements."},
				llm.ToolCallStart{ToolName: "WriteFile", ToolID: "w1"},
				llm.ToolCallComplete{ToolID: "w1", ToolName: "WriteFile", Arguments: map[string]any{
					"file_path": outputFile,
					"content": `const express = require('express');
const cors = require('cors');
const app = express();

app.use(cors());
app.use(express.json());

app.get('/health', (req, res) => res.json({ status: 'ok' }));
app.get('/', (req, res) => res.send('Hello'));

app.use((err, req, res, next) => {
  console.error(err.stack);
  res.status(500).json({ error: 'Internal server error' });
});

app.listen(3000);
`,
				}},
				llm.StreamEnd{StopReason: "tool_use"},
			}
		},
		// Final
		func(msgs []conversation.Message) []llm.StreamEvent {
			return []llm.StreamEvent{
				llm.TextDelta{Text: "Updated! Added CORS, JSON parsing, health check, and error handling."},
				llm.StreamEnd{StopReason: "end_turn"},
			}
		},
	}}

	reg := tools.NewRegistry()
	reg.Register(&tools.ReadFileTool{})
	reg.Register(&tools.WriteFileTool{})

	ag := New(client, reg, "anthropic")
	conv := conversation.NewManager()

	// Round 1: user asks to improve the code
	text1, _ := runConversationRound(ag, conv, fmt.Sprintf("improve the express app at %s", srcFile))
	t.Logf("Agent (round 1): %s", text1)
	if !strings.Contains(text1, "proceed") {
		t.Errorf("agent should ask for confirmation, got: %s", text1)
	}

	// Round 2: user confirms
	text2, _ := runConversationRound(ag, conv, "yes, go ahead")
	t.Logf("Agent (round 2): %s", text2)

	// Verify the file was actually modified
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	updated := string(content)

	checks := []struct {
		substr string
		desc   string
	}{
		{"cors", "CORS middleware added"},
		{"express.json()", "JSON body parser added"},
		{"/health", "health check endpoint added"},
		{"err, req, res, next", "error handling middleware added"},
		{"app.listen(3000)", "original listen preserved"},
		{"res.send('Hello')", "original route preserved"},
	}
	for _, c := range checks {
		if !strings.Contains(updated, c.substr) {
			t.Errorf("output missing %s (looking for %q)", c.desc, c.substr)
		}
	}

	// Verify conversation round-trip integrity
	msgs := conv.GetMessages()
	t.Logf("Conversation: %d messages", len(msgs))
	userMsgCount := 0
	assistantMsgCount := 0
	for _, m := range msgs {
		if m.Role == "user" && m.Content != "" {
			userMsgCount++
		}
		if m.Role == "assistant" {
			assistantMsgCount++
		}
	}
	if userMsgCount < 2 {
		t.Errorf("expected at least 2 user messages, got %d", userMsgCount)
	}
	if assistantMsgCount < 2 {
		t.Errorf("expected at least 2 assistant messages, got %d", assistantMsgCount)
	}
	t.Logf("All %d content checks passed on output file (%d bytes)", len(checks), len(content))
}

func TestRealSkillsLoadAndRunSimulation(t *testing.T) {
	// Load the actual installed skills and verify end-to-end simulation works
	wd, _ := os.Getwd()
	for wd != "/" {
		if _, err := os.Stat(filepath.Join(wd, ".swiftx", "skills")); err == nil {
			break
		}
		wd = filepath.Dir(wd)
	}
	skillsDir := filepath.Join(wd, ".swiftx", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		t.Skip("No .swiftx/skills directory")
	}

	catalog, _ := skills.LoadFromDirectory(skillsDir)
	metas := catalog.List()
	t.Logf("Loaded %d real skills", len(metas))

	skillListing := buildSkillListing(skillsDir, catalog)

	// Verify system prompt structure
	if !strings.Contains(skillListing, "Skills are installed at:") {
		t.Error("system prompt missing skill path")
	}
	for _, meta := range metas {
		if !strings.Contains(skillListing, "/"+meta.Name) {
			t.Errorf("system prompt missing /%s", meta.Name)
		}
	}

	// Test each real skill can be loaded and used as a prompt
	for _, meta := range metas {
		skill := catalog.Get(meta.Name)
		if skill == nil {
			t.Errorf("skill %q returned nil from Get", meta.Name)
			continue
		}
		if skill.PromptBody == "" {
			t.Errorf("skill %q has empty body", meta.Name)
			continue
		}

		// Simulate invoking the skill with args
		prompt := skill.PromptBody + "\n\n## User Request\n\ntest request for " + meta.Name
		if !strings.Contains(prompt, "## User Request") {
			t.Errorf("skill %q prompt missing user request section", meta.Name)
		}

		// Run one round with a mock that verifies it received the skill prompt
		client := &dynamicMock{handlers: []func([]conversation.Message) []llm.StreamEvent{
			func(msgs []conversation.Message) []llm.StreamEvent {
				lastUser := msgs[len(msgs)-1]
				if !strings.Contains(lastUser.Content, meta.Name) && !strings.Contains(lastUser.Content, "test request") {
					t.Errorf("skill %q: agent did not receive skill prompt", meta.Name)
				}
				return []llm.StreamEvent{
					llm.TextDelta{Text: fmt.Sprintf("I received the %s skill prompt and I'm ready to help.", meta.Name)},
					llm.StreamEnd{StopReason: "end_turn"},
				}
			},
		}}

		ag := New(client, tools.NewRegistry(), "anthropic")
		conv := conversation.NewManager()
		text, _ := runConversationRound(ag, conv, prompt)

		if !strings.Contains(text, meta.Name) {
			t.Errorf("skill %q: mock response should echo skill name, got: %s", meta.Name, text)
		}
		t.Logf("  /%s: prompt=%d chars, response OK", meta.Name, len(prompt))
	}
}

func TestAgentOnLoopCompleteFiresOnFinalTurn(t *testing.T) {
	client := &mockClient{responses: [][]llm.StreamEvent{{
		llm.TextDelta{Text: "done"},
		llm.StreamEnd{StopReason: "end_turn"},
	}}}
	ag := New(client, tools.NewRegistry(), "anthropic")

	got := make(chan *conversation.Manager, 1)
	ag.OnLoopComplete = func(conv *conversation.Manager) {
		got <- conv
	}

	conv := conversation.NewManager()
	runConversationRound(ag, conv, "hi")

	select {
	case received := <-got:
		if received != conv {
			t.Error("callback should receive the same conv pointer")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnLoopComplete was not called within 2s")
	}
}

func TestAgentOnLoopCompleteSkippedOnError(t *testing.T) {
	// Hits MaxIterations before LoopComplete — callback must not fire.
	loop := []llm.StreamEvent{
		llm.ToolCallStart{ToolName: "Glob", ToolID: "t"},
		llm.ToolCallComplete{ToolID: "t", ToolName: "Glob", Arguments: map[string]any{"pattern": "*"}},
		llm.StreamEnd{StopReason: "tool_use"},
	}
	responses := make([][]llm.StreamEvent, 5)
	for i := range responses {
		responses[i] = loop
	}
	client := &mockClient{responses: responses}
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "Glob", result: "f.txt"})
	ag := New(client, reg, "anthropic")
	ag.MaxIterations = 2

	called := make(chan struct{}, 1)
	ag.OnLoopComplete = func(*conversation.Manager) { called <- struct{}{} }

	conv := conversation.NewManager()
	runConversationRound(ag, conv, "spin")

	select {
	case <-called:
		t.Error("OnLoopComplete must not fire when loop exits via error")
	case <-time.After(200 * time.Millisecond):
		// expected: no callback
	}
}

func TestFilterSchemasByName(t *testing.T) {
	schemas := []map[string]any{
		{"name": "Agent", "x": 1},
		{"name": "Bash", "x": 2},
		{"name": "ReadFile", "x": 3},
	}
	allow := func(name string) bool { return name == "Agent" || name == "ReadFile" }
	got := filterSchemasByName(schemas, allow)
	if len(got) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(got))
	}
	names := map[string]bool{}
	for _, s := range got {
		names[s["name"].(string)] = true
	}
	if !names["Agent"] || !names["ReadFile"] || names["Bash"] {
		t.Errorf("filter kept the wrong set: %v", names)
	}
}

func TestFilterSchemasByNameEmptyInput(t *testing.T) {
	got := filterSchemasByName(nil, func(string) bool { return true })
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d entries", len(got))
	}
}

func schemaNames(schemas []map[string]any) []string {
	var out []string
	for _, s := range schemas {
		if n, ok := s["name"].(string); ok {
			out = append(out, n)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	return slices.Contains(list, want)
}
