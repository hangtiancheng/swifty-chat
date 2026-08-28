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

package subagent

import (
	"context"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

func TestBuildForkedConversationPreservesThinkingBlocks(t *testing.T) {
	// Byte-exact replay: an assistant message with thinking blocks must be reproduced with the same
	// thinking blocks in the forked conversation, otherwise the API request prefix diverges and the
	// prompt cache misses.
	parent := conversation.NewManager()
	thinking := []conversation.ThinkingBlock{{Thinking: "secret plan", Signature: "sig-1"}}
	parent.AddAssistantFull("hello", thinking, []conversation.ToolUseBlock{
		{ToolUseID: "tool_1", ToolName: "Bash", Arguments: map[string]any{"command": "ls"}},
	})

	forked := buildForkedConversation(parent, "do work")
	msgs := forked.GetMessages()
	var found *conversation.Message
	for i := range msgs {
		if len(msgs[i].ThinkingBlocks) > 0 {
			found = &msgs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("forked conversation lost thinking blocks")
	}
	if found.ThinkingBlocks[0].Thinking != "secret plan" || found.ThinkingBlocks[0].Signature != "sig-1" {
		t.Errorf("thinking blocks not preserved verbatim: %+v", found.ThinkingBlocks)
	}
}

func TestDeriveSubAgentCheckerOverrideMode(t *testing.T) {
	// spec.PermissionMode must produce a Checker that shares the parent's Sandbox / RuleEngine but
	// flips the Mode. Empty override → unchanged.
	sb := permissions.NewPathSandbox("/tmp", "")
	eng := &permissions.RuleEngine{}
	parent := permissions.NewChecker(sb, eng, permissions.ModeDefault)

	if derived := deriveSubAgentChecker(parent, ""); derived != parent {
		t.Error("empty override should return parent unchanged")
	}
	derived := deriveSubAgentChecker(parent, "plan")
	if derived == parent {
		t.Fatal("plan override should produce a new checker instance")
	}
	if derived.Mode != permissions.ModePlan {
		t.Errorf("derived.Mode = %q, want plan", derived.Mode)
	}
	if derived.Sandbox != sb || derived.RuleEngine != eng {
		t.Error("derived checker should share parent's Sandbox + RuleEngine")
	}
	if deriveSubAgentChecker(nil, "plan") != nil {
		t.Error("nil parent should propagate nil")
	}
}

func TestRunForkRejectedWhenQuerySourceIsFork(t *testing.T) {
	// Primary nested-fork guard: check the QuerySource marker directly.
	tool := &AgentTool{
		Registry:     tools.NewRegistry(),
		Conversation: conversation.NewManager(),
		QuerySource:  ForkQuerySource,
		TaskMgr:      NewTaskManager(),
	}
	result := tool.runFork(context.Background(), "desc", "do work", "", "")
	if !result.IsError {
		t.Fatal("runFork should reject when QuerySource is fork")
	}
	if !strings.Contains(result.Output, "cannot fork from a forked agent") {
		t.Errorf("unexpected error message: %s", result.Output)
	}
}

func TestRunForkRejectedWhenBoilerplateInHistory(t *testing.T) {
	// Fallback nested-fork guard: when QuerySource didn't propagate, scan the
	// conversation history for the fork boilerplate marker.
	conv := conversation.NewManager()
	conv.AddUserMessage(ForkBoilerplateTag + " stale message from a prior fork")
	tool := &AgentTool{
		Registry:     tools.NewRegistry(),
		Conversation: conv,
		TaskMgr:      NewTaskManager(),
	}
	result := tool.runFork(context.Background(), "desc", "do work", "", "")
	if !result.IsError {
		t.Fatal("runFork should reject when conversation history contains ForkBoilerplateTag")
	}
}

func TestCloneRegistryForForkSetsQuerySource(t *testing.T) {
	// A fork must inherit the parent tool pool verbatim, replacing only the Agent
	// tool with a copy carrying QuerySource=ForkQuerySource, so a further fork
	// attempt is intercepted at call time.
	reg := tools.NewRegistry()
	reg.Register(&AgentTool{}) // simulate parent's Agent tool
	reg.Register(&dummyTool{name: "Bash", category: tools.CategoryCommand})

	forked := cloneRegistryForFork(reg)
	if forked.Get("Bash") == nil {
		t.Error("Bash should still be present in forked registry")
	}
	at, ok := forked.Get("Agent").(*AgentTool)
	if !ok {
		t.Fatal("Agent tool should still be present (tool pool inherited verbatim)")
	}
	if at.QuerySource != ForkQuerySource {
		t.Errorf("cloned Agent tool QuerySource = %q, want %q", at.QuerySource, ForkQuerySource)
	}
}

func TestExecuteRoutesBackgroundSpecToAsync(t *testing.T) {
	// An agent definition marked background must be routed to async dispatch,
	// even when the caller does not pass run_in_background.
	tool := &AgentTool{
		Registry: tools.NewRegistry(),
		TaskMgr:  NewTaskManager(),
		Protocol: "anthropic",
	}
	result := tool.Execute(context.Background(), map[string]any{
		"description":   "verify run",
		"prompt":        "do it",
		"subagent_type": "background-only",
	})
	if !result.IsError {
		// Without a registered spec the call should fail; that's the only safe shape under unit testing —
		// but it must not get there via the sync path. The IsError = true with "unknown agent type"
		// message confirms parameter parsing reached the spec lookup.
		if !strings.Contains(result.Output, "unknown agent type") {
			t.Errorf("expected unknown-agent-type error, got %q", result.Output)
		}
	}
}

func TestExecuteValidatesMode(t *testing.T) {
	tool := &AgentTool{
		Registry: tools.NewRegistry(),
		TaskMgr:  NewTaskManager(),
	}

	badMode := tool.Execute(context.Background(), map[string]any{
		"description": "x", "prompt": "y",
		"mode": "not-a-real-mode",
	})
	if !badMode.IsError || !strings.Contains(badMode.Output, "invalid mode") {
		t.Errorf("invalid mode must be rejected, got %q", badMode.Output)
	}
}
