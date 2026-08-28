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

// CoordinatorMode narrows the Lead's tool set to pure scheduling.
//
// The dividing line is not "read" vs "write" but whether a tool floods the
// Lead's context with large content. The Lead's context must hold task
// decomposition, teammate status, and message history; once it can read files
// or run commands directly, the model will inevitably investigate on its own —
// thousands of lines of code pour in, leaving no room for actual scheduling.
// Therefore ReadFile / Glob / Grep / Bash are excluded: delegate code
// inspection to teammates, who bring back conclusions for the Lead to digest
// and turn into the next specification.
//
// Task assignment to teammates is conveyed through the Agent prompt, not a
// shared task board, so TaskCreate / TaskGet / TaskList / TaskUpdate are also
// withheld from the Lead; those are coordination tools registered in the
// teammate tool set (see agents.AgentTool.runAsTeammate). The Lead tracks
// progress via <task-notification> messages returned when teammates finish.
//
// TeamDelete is retained for teardown: teammates hang off the Team, and there
// must be a way to stop them and clean up the team directory when work is done.
// TeamCreate is excluded because the Agent tool auto-creates a Team when the
// specified one does not exist — the Lead can dispatch directly without a
// separate team-creation step.
//
// Four-phase workflow:
// 1. Research: teammates investigate in parallel; the Lead does not participate
// 2. Synthesis: the Lead digests findings and writes an implementation spec
// 3. Implementation: teammates modify code per the spec and commit
// 4. Verification: teammates verify the changes are correct
var CoordinatorAllowedTools = map[string]bool{
	"Agent":           true,
	"SendMessage":     true,
	"TaskStop":        true,
	"SyntheticOutput": true,
	"TeamDelete":      true,
}

// IsCoordinatorTool checks if a tool is allowed in Coordinator Mode.
func IsCoordinatorTool(name string) bool {
	return CoordinatorAllowedTools[name]
}

// CoordinatorToolFilter returns the tool filter function for the Lead Agent.
// When enabled is false it returns nil (no restriction); when true, only
// whitelisted tools are allowed for the entire session.
//
// The decision is based solely on configuration, not on whether a team exists:
// switching modes mid-session leaves stale scheduling instructions in the
// conversation history that cannot be retracted, and the model would continue
// following outdated constraints. Configuration is authoritative from the first
// turn to the last.
func CoordinatorToolFilter(enabled bool) func(name string) bool {
	if !enabled {
		return nil
	}
	return IsCoordinatorTool
}

// CoordinatorActiveFn returns a function that reports whether coordinator mode
// is active, using the same criteria as the tool filter.
// Scheduling instructions and tool narrowing must appear together: narrowing
// tools without instructions would leave the Lead unable to read files but
// unaware that it should delegate reading to teammates.
func CoordinatorActiveFn(enabled bool) func() bool {
	if !enabled {
		return nil
	}
	return func() bool { return true }
}
