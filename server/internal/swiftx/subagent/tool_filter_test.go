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
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

type dummyTool struct {
	name     string
	category tools.ToolCategory
}

func (d *dummyTool) Name() string                 { return d.name }
func (d *dummyTool) Description() string          { return "test tool" }
func (d *dummyTool) Category() tools.ToolCategory { return d.category }

func (d *dummyTool) Schema() map[string]any { return nil }
func (d *dummyTool) Execute(_ context.Context, _ map[string]any) tools.ToolResult {
	return tools.ToolResult{}
}

func makeRegistry(names ...string) *tools.Registry {
	reg := tools.NewRegistry()
	for _, n := range names {
		reg.Register(&dummyTool{name: n, category: tools.CategoryRead})
	}
	return reg
}

func hasToolNamed(reg *tools.Registry, name string) bool {
	return reg.Get(name) != nil
}

func TestFilterRemovesAgentTool(t *testing.T) {
	reg := makeRegistry("ReadFile", "Agent", "Bash")
	filtered := FilterToolsForAgent(reg, nil, nil, false)
	if hasToolNamed(filtered, "Agent") {
		t.Error("Agent tool should be removed from sub-agent registry")
	}
	if !hasToolNamed(filtered, "ReadFile") {
		t.Error("ReadFile should remain")
	}
	if !hasToolNamed(filtered, "Bash") {
		t.Error("Bash should remain")
	}
}

func TestFilterRemovesAskUserQuestion(t *testing.T) {
	reg := makeRegistry("ReadFile", "AskUserQuestion")
	filtered := FilterToolsForAgent(reg, nil, nil, false)
	if hasToolNamed(filtered, "AskUserQuestion") {
		t.Error("AskUserQuestion should be removed from sub-agent registry")
	}
}

func TestAsyncFilterWhitelist(t *testing.T) {
	reg := makeRegistry("ReadFile", "WriteFile", "EditFile", "Glob", "Grep", "Bash", "ToolSearch", "Agent", "AskUserQuestion", "TaskCreate", "TaskList")
	filtered := FilterToolsForAgent(reg, nil, nil, true)

	allowed := []string{"ReadFile", "WriteFile", "EditFile", "Glob", "Grep", "Bash", "ToolSearch"}
	for _, name := range allowed {
		if !hasToolNamed(filtered, name) {
			t.Errorf("%s should be allowed for async agents", name)
		}
	}

	blocked := []string{"Agent", "AskUserQuestion", "TaskCreate", "TaskList"}
	for _, name := range blocked {
		if hasToolNamed(filtered, name) {
			t.Errorf("%s should be blocked for async agents", name)
		}
	}
}

func TestMCPToolsPassThrough(t *testing.T) {
	reg := makeRegistry("mcp__grafana__query", "Agent", "ReadFile")
	filtered := FilterToolsForAgent(reg, nil, nil, true)
	if !hasToolNamed(filtered, "mcp__grafana__query") {
		t.Error("MCP tools should always pass through filter")
	}
}

func TestGlobalDisallowedExpanded(t *testing.T) {
	// Each of these must be blocked for every sub-agent regardless of definition allowlist.
	reg := makeRegistry(
		"ReadFile",
		"TaskOutput",
		"ExitPlanMode",
		"EnterPlanMode",
		"Agent",
		"AskUserQuestion",
		"TaskStop",
		"Workflow",
	)
	filtered := FilterToolsForAgent(reg, []string{"*"}, nil, false)
	for _, blocked := range []string{
		"TaskOutput", "ExitPlanMode", "EnterPlanMode", "Agent", "AskUserQuestion", "TaskStop", "Workflow",
	} {
		if hasToolNamed(filtered, blocked) {
			t.Errorf("%s should be in ALL_AGENT_DISALLOWED_TOOLS", blocked)
		}
	}
	if !hasToolNamed(filtered, "ReadFile") {
		t.Error("ReadFile should remain")
	}
}

func TestAsyncWhitelistExpanded(t *testing.T) {
	// Async agents may only use this whitelisted set of tools.
	reg := makeRegistry(
		"ReadFile", "WebSearch", "TodoWrite", "Grep", "WebFetch", "Glob",
		"Bash", "EditFile", "WriteFile", "NotebookEdit", "Skill",
		"SyntheticOutput", "ToolSearch", "EnterWorktree", "ExitWorktree",
	)
	filtered := FilterToolsForAgent(reg, nil, nil, true)
	for _, name := range []string{
		"ReadFile", "WebSearch", "TodoWrite", "Grep", "WebFetch", "Glob",
		"Bash", "EditFile", "WriteFile", "NotebookEdit", "Skill",
		"SyntheticOutput", "ToolSearch", "EnterWorktree", "ExitWorktree",
	} {
		if !hasToolNamed(filtered, name) {
			t.Errorf("%s should be allowed for async agents", name)
		}
	}
}

func TestInProcessTeammateExtraTools(t *testing.T) {
	// Coordination tools that are normally blocked by the async whitelist must be allowed when the
	// sub-agent is an in-process teammate.
	reg := makeRegistry("ReadFile", "TaskCreate", "TaskList", "SendMessage", "Agent")
	asTeammate := FilterToolsForAgentEx(reg, nil, nil, true, false, true)
	for _, name := range []string{"TaskCreate", "TaskList", "SendMessage"} {
		if !hasToolNamed(asTeammate, name) {
			t.Errorf("%s should be allowed for in-process teammates", name)
		}
	}
	// Agent tool is allowed for in-process teammates (to spawn sync subagents)
	// but the global ALL_AGENT_DISALLOWED_TOOLS gate runs before the teammate
	// check, so it still gets blocked. Document the current behavior.
	notTeammate := FilterToolsForAgentEx(reg, nil, nil, true, false, false)
	for _, name := range []string{"TaskCreate", "TaskList", "SendMessage"} {
		if hasToolNamed(notTeammate, name) {
			t.Errorf("%s should be blocked for plain async agents (not teammates)", name)
		}
	}
}

func TestDisallowedToolsApplied(t *testing.T) {
	reg := makeRegistry("ReadFile", "EditFile", "WriteFile", "Bash")
	filtered := FilterToolsForAgent(reg, nil, []string{"EditFile", "WriteFile"}, false)
	if hasToolNamed(filtered, "EditFile") {
		t.Error("EditFile should be blocked by disallowedTools")
	}
	if hasToolNamed(filtered, "WriteFile") {
		t.Error("WriteFile should be blocked by disallowedTools")
	}
	if !hasToolNamed(filtered, "ReadFile") {
		t.Error("ReadFile should remain")
	}
	if !hasToolNamed(filtered, "Bash") {
		t.Error("Bash should remain")
	}
}

func TestGeneralPurposeNoRecursion(t *testing.T) {
	reg := makeRegistry("ReadFile", "Agent", "Bash", "EditFile", "WriteFile", "Glob", "Grep", "ToolSearch", "AskUserQuestion")
	spec := BuiltinSpecs["general-purpose"]
	filtered := FilterToolsForAgent(reg, spec.Tools, spec.DisallowedTools, false)
	if hasToolNamed(filtered, "Agent") {
		t.Error("general-purpose sub-agent should NOT have Agent tool (prevents infinite recursion)")
	}
	if hasToolNamed(filtered, "AskUserQuestion") {
		t.Error("general-purpose sub-agent should NOT have AskUserQuestion")
	}
	if !hasToolNamed(filtered, "ReadFile") {
		t.Error("ReadFile should remain for general-purpose")
	}
	if !hasToolNamed(filtered, "EditFile") {
		t.Error("EditFile should remain for general-purpose (sync)")
	}
}

// An in-process teammate's tool set is filtered down from the lead's copy; the
// disallow list must be the spec's own list merged with
// TeammateDisallowedTools, otherwise teammates would inherit team membership
// management along with everything else.
func TestTeammateFilterBlocksTeamManagement(t *testing.T) {
	reg := makeRegistry("ReadFile", "Bash", "EditFile", "Agent", "TeamCreate", "TeamDelete", "SendMessage")
	spec := BuiltinSpecs["general-purpose"]

	disallowed := append(append([]string{}, spec.DisallowedTools...), TeammateDisallowedTools...)
	filtered := FilterToolsForAgent(reg, spec.Tools, disallowed, false)

	for _, name := range []string{"Agent", "TeamCreate", "TeamDelete"} {
		if hasToolNamed(filtered, name) {
			t.Errorf("teammate tool set should not include %s", name)
		}
	}
	for _, name := range []string{"ReadFile", "Bash", "EditFile"} {
		if !hasToolNamed(filtered, name) {
			t.Errorf("teammate tool set is missing %s", name)
		}
	}
}
