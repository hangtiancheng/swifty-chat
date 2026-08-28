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

package teams

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/prompt"
)

// The Lead in coordinator mode only schedules; it never touches code directly.
func TestCoordinatorBlocksCodeTools(t *testing.T) {
	blocked := []string{"ReadFile", "WriteFile", "EditFile", "Glob", "Grep", "Bash"}
	for _, name := range blocked {
		if IsCoordinatorTool(name) {
			t.Errorf("%s should not appear in the coordinator tool set; reading and modifying code should be delegated to teammates", name)
		}
	}
}

// The task board is for coordination among teammates; the Lead tracks progress via task-notification.
func TestCoordinatorBlocksTaskBoardTools(t *testing.T) {
	for _, name := range []string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate"} {
		if IsCoordinatorTool(name) {
			t.Errorf("%s is a teammate coordination tool and should not be given to the Lead", name)
		}
	}
}

func TestCoordinatorAllowsSchedulingTools(t *testing.T) {
	for _, name := range []string{"Agent", "SendMessage", "TaskStop", "SyntheticOutput"} {
		if !IsCoordinatorTool(name) {
			t.Errorf("%s is essential for scheduling; without it the Lead cannot function", name)
		}
	}
}

// TeamDelete is the only entry point for tearing down a Team, and coordinator
// mode is triggered by "whether a Team exists". Blocking it would leave the
// Lead permanently stuck in coordinator mode after creating a Team.
func TestCoordinatorKeepsTeamDeleteToAvoidLockIn(t *testing.T) {
	if !IsCoordinatorTool("TeamDelete") {
		t.Fatal("TeamDelete must be allowed, otherwise the Lead gets locked in coordinator mode")
	}
}

func TestCoordinatorToolFilterIsStatic(t *testing.T) {
	filter := CoordinatorToolFilter(true)
	if filter == nil {
		t.Fatal("should return a filter when enabled is true")
	}

	// Configuration is authoritative: narrowing applies from the first turn,
	// regardless of whether a team exists.
	if filter("Bash") || filter("ReadFile") || filter("TeamCreate") {
		t.Error("once enabled, only the whitelist should pass, regardless of team existence")
	}
	if !filter("Agent") || !filter("SendMessage") {
		t.Error("scheduling tools should always be allowed")
	}
}

// Scheduling instructions and tool narrowing must take effect simultaneously:
// narrowing without instructions would leave the Lead unable to read files but
// unaware that it should delegate reading to teammates.
func TestCoordinatorActiveFnTracksToolFilter(t *testing.T) {
	filter := CoordinatorToolFilter(true)
	active := CoordinatorActiveFn(true)
	if active == nil {
		t.Fatal("should return a predicate function when enabled is true")
	}
	if !active() {
		t.Error("should always be active once enabled")
	}
	// Both predicates must agree, otherwise we get a state where "tools are
	// narrowed but no instructions are injected".
	if active() != !filter("Bash") {
		t.Error("instruction injection and tool narrowing predicates are inconsistent")
	}
}

func TestCoordinatorDisabledReturnsNil(t *testing.T) {
	if CoordinatorToolFilter(false) != nil {
		t.Error("should not return a filter when disabled")
	}
	if CoordinatorActiveFn(false) != nil {
		t.Error("should not return a predicate function when disabled")
	}
}

// TaskStop stops teammates, not entries in the background task board: in
// coordinator mode the Lead dispatches teammates via the Agent tool with
// team_name, and they do not exist in the background task board.
func TestTaskStopStopsTeammate(t *testing.T) {
	useTempHome(t)
	mgr := NewTeamManager()
	team := mgr.CreateTeam("squad", ModeInProcess)
	member := team.AddMember("scout", nil, nil, "anthropic")
	member.Active = true
	stopped := false
	member.Cancel = func() { stopped = true }

	tool := &TaskStopTool{TeamMgr: mgr}
	res := tool.Execute(context.Background(), map[string]any{"teammate": "scout"})
	if res.IsError {
		t.Fatalf("stopping a running teammate should not error: %s", res.Output)
	}
	if !stopped {
		t.Error("the teammate's Cancel was not called")
	}
	if member.Active {
		t.Error("Active should be false after stopping")
	}
}

func TestTaskStopOnUnknownTeammate(t *testing.T) {
	useTempHome(t)
	mgr := NewTeamManager()
	mgr.CreateTeam("squad", ModeInProcess)

	tool := &TaskStopTool{TeamMgr: mgr}
	res := tool.Execute(context.Background(), map[string]any{"teammate": "ghost"})
	if !res.IsError {
		t.Error("stopping a non-existent teammate should return an error")
	}
}

// Stopping an already-idle teammate should not error, to avoid the model
// retrying endlessly on the error.
func TestTaskStopOnIdleTeammate(t *testing.T) {
	useTempHome(t)
	mgr := NewTeamManager()
	team := mgr.CreateTeam("squad", ModeInProcess)
	team.AddMember("scout", nil, nil, "anthropic")

	tool := &TaskStopTool{TeamMgr: mgr}
	res := tool.Execute(context.Background(), map[string]any{"teammate": "scout"})
	if res.IsError {
		t.Errorf("stopping an already-idle teammate should not error: %s", res.Output)
	}
}

// Swiftx's built-in types are general-purpose / plan / explore; there is no "worker".
func TestCoordinatorPromptUsesRealSubagentType(t *testing.T) {
	p := prompt.CoordinatorReminder(1)
	if strings.Contains(p, `subagent_type: "worker"`) || strings.Contains(p, "subagent_type `worker`") {
		t.Error("prompt references a non-existent subagent_type")
	}
}

// The tools listed in the prompt must exactly match the whitelist, otherwise
// the model will attempt to call filtered-out tools.
func TestCoordinatorPromptListsOnlyAllowedTools(t *testing.T) {
	p := prompt.CoordinatorReminder(1)
	toolsSection := p[strings.Index(p, "## 2. Your Tools"):strings.Index(p, "### Worker Results")]
	for name := range CoordinatorAllowedTools {
		if !strings.Contains(toolsSection, "**"+name+"**") {
			t.Errorf("whitelisted tool %s is missing from the prompt's tool list", name)
		}
	}
	for _, name := range []string{"ReadFile", "Bash", "Grep", "TaskCreate", "TeamCreate"} {
		if strings.Contains(toolsSection, "**"+name+"**") {
			t.Errorf("prompt's tool list includes filtered-out tool %s", name)
		}
	}
}

// The response format described in the instructions must match what
// DrainLeadMailbox actually delivers, otherwise the Lead will look for
// teammate names in a field that does not exist.
func TestCoordinatorPromptMatchesTeamNotificationFormat(t *testing.T) {
	p := prompt.CoordinatorReminder(1)
	if !strings.Contains(p, "<team-notification") || !strings.Contains(p, "from=") {
		t.Error("instructions should describe the real <team-notification> and from= format")
	}
	// <task_id> is the channel for background sub-agents; the Lead does not
	// use it in coordinator mode.
	if strings.Contains(p, "<task_id>") {
		t.Error("instructions describe the background sub-agent channel, not the teammate response channel")
	}
}

// This instruction is just over 8KB, and AddSystemReminder is a pure append;
// resending it verbatim every turn would refill the context this mode saves.
func TestCoordinatorReminderGoesSparseAfterFirstTurn(t *testing.T) {
	full := prompt.CoordinatorReminder(1)
	second := prompt.CoordinatorReminder(2)
	if len(second) >= len(full) {
		t.Fatalf("second turn should send a condensed version, got %d bytes vs first turn %d bytes", len(second), len(full))
	}
	// The condensed version must still enforce the most easily forgotten hard constraints.
	for _, must := range []string{"cannot read files", "TaskStop", "from="} {
		if !strings.Contains(second, must) {
			t.Errorf("condensed version lost a critical constraint: %s", must)
		}
	}
	// Periodically restate the full text to prevent drift in long sessions.
	var sawFull bool
	for i := 2; i <= 12; i++ {
		if prompt.CoordinatorReminder(i) == full {
			sawFull = true
			break
		}
	}
	if !sawFull {
		t.Error("should periodically restate the full text in long sessions")
	}
}

// All three entry points (TUI / remote / print) must install team tools and
// share the same coordinator predicates, otherwise the same feature behaves
// inconsistently across entry points: some can dispatch teammates while others
// cannot even after creating a team.
func TestCoordinatorWiringIsSameAcrossEntrypoints(t *testing.T) {
	roots := map[string]string{
		"tui":    "../tui/tui.go",
		"remote": "../remote/server.go",
		"print":  "../../cmd/swiftx/print.go",
	}
	// Assembly fragments that must appear in every entry point.
	required := []string{
		"teams.TeamCreateTool",
		"teams.TeamDeleteTool",
		"teams.TaskStopTool",
		"tools.SyntheticOutputTool",
		"CoordinatorToolFilter",
		"CoordinatorActiveFn",
		"DrainLeadMailbox",
	}
	for entry, path := range roots {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read source for %s: %v", entry, err)
		}
		for _, want := range required {
			if !strings.Contains(string(src), want) {
				t.Errorf("%s entry point is missing %s; coordinator is incomplete at this entry point", entry, want)
			}
		}
	}
}

// In coordinator mode TeamCreate is not in the whitelist; the Agent tool must
// be able to create the team itself, otherwise the Lead gets stuck trying to
// dispatch the first teammate.
func TestTeamCreateNotNeededUnderCoordinator(t *testing.T) {
	if IsCoordinatorTool("TeamCreate") {
		t.Error("TeamCreate should not be in the whitelist; the Agent tool auto-creates teams")
	}
	// Teardown relies on TeamDelete: teammates hang off the Team and must be stoppable.
	if !IsCoordinatorTool("TeamDelete") {
		t.Error("TeamDelete should be retained, otherwise the team cannot be torn down")
	}
}
