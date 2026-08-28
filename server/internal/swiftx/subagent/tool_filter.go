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
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// AllAgentDisallowedTools lists tools that no sub-agent may use; even an agent
// definition's allowlist cannot enable them. TaskOutput, ExitPlanMode,
// EnterPlanMode, and Workflow have no implementation in the current tool set —
// they are kept so this list fully spells out which capabilities sub-agents
// must never have. The filter skips unknown names harmlessly, and the entries
// take effect automatically once tools with those names are added.
var AllAgentDisallowedTools = map[string]bool{
	"TaskOutput":      true,
	"ExitPlanMode":    true,
	"EnterPlanMode":   true,
	"Agent":           true,
	"AskUserQuestion": true,
	"TaskStop":        true,
	"Workflow":        true,
}

// CustomAgentDisallowedTools , which is just a clone of ALL_AGENT_DISALLOWED_TOOLS. Kept as a
// separate map so future extra restrictions can be added without touching the global list.
var CustomAgentDisallowedTools = map[string]bool{
	"TaskOutput":      true,
	"ExitPlanMode":    true,
	"EnterPlanMode":   true,
	"Agent":           true,
	"AskUserQuestion": true,
	"TaskStop":        true,
	"Workflow":        true,
}

// AsyncAgentAllowedTools Async (background) agents can only use these tools — no Agent (no nested
// spawn), no TaskOutput, no ExitPlanMode, no TaskStop. Local tool naming maps FILE_READ → ReadFile,
// FILE_EDIT → EditFile, FILE_WRITE → WriteFile.
var AsyncAgentAllowedTools = map[string]bool{
	"ReadFile":        true,
	"WebSearch":       true,
	"TodoWrite":       true,
	"Grep":            true,
	"WebFetch":        true,
	"Glob":            true,
	"Bash":            true,
	"EditFile":        true,
	"WriteFile":       true,
	"NotebookEdit":    true,
	"Skill":           true,
	"LoadSkill":       true,
	"SyntheticOutput": true,
	"ToolSearch":      true,
	"EnterWorktree":   true,
	"ExitWorktree":    true,
	// ToolSearch only reads schemas; actual invocation requires McpCall.
	// Both must be allowed together, otherwise the sub-agent can see tools
	// but cannot call them.
	"McpCall": true,
}

// InProcessTeammateAllowedTools When a sub-agent is spawned as an in-process teammate (ch15 Agent
// Teams), it gets the async whitelist plus these coordination tools so it can manage the shared
// task list and send messages to peers.
var InProcessTeammateAllowedTools = map[string]bool{
	"TaskCreate":  true,
	"TaskGet":     true,
	"TaskList":    true,
	"TaskUpdate":  true,
	"SendMessage": true,
	"CronCreate":  true,
	"CronDelete":  true,
	"CronList":    true,
}

// TeammateDisallowedTools lists tools blocked for teammates on top of the
// collaboration whitelist. Forming and disbanding teams is the lead's
// responsibility; teammates only do the work and coordinate with each other,
// and take no part in team membership management.
var TeammateDisallowedTools = []string{"TeamCreate", "TeamDelete"}

func IsMCPTool(name string) bool {
	return strings.HasPrefix(name, "mcp__")
}

func FilterToolsForAgent(reg *tools.Registry, allowedTools, disallowedTools []string, isAsync bool) *tools.Registry {
	return FilterToolsForAgentEx(reg, allowedTools, disallowedTools, isAsync, false, false)
}

// FilterToolsForAgentEx
//
// Layers applied in order: 1. MCP tools (mcp__*) — always allowed 2. ALL_AGENT_DISALLOWED_TOOLS —
// global block (recursion / main-thread only) 3. CUSTOM_AGENT_DISALLOWED_TOOLS — custom (non-built-
// in) agents only 4. ASYNC_AGENT_ALLOWED_TOOLS — background agents are whitelisted; if the agent is
// an in-process teammate, also allow IN_PROCESS_TEAMMATE_ALLOWED_TOOLS 5. Agent definition
// disallowedTools — definition-level blacklist 6. Agent definition tools — definition-level
// whitelist intersection ("*" disables this).
//
// isCustom: agent loaded from .swiftx/agents/, not a built-in. isInProcessTeammate: spawned via
// TeamCreate / SpawnTeammate in ch15.
func FilterToolsForAgentEx(reg *tools.Registry, allowedTools, disallowedTools []string, isAsync, isCustom, isInProcessTeammate bool) *tools.Registry {
	disallowed := make(map[string]bool, len(disallowedTools))
	for _, name := range disallowedTools {
		disallowed[name] = true
	}

	allowed := make(map[string]bool, len(allowedTools))
	hasWhitelist := len(allowedTools) > 0 && (len(allowedTools) != 1 || allowedTools[0] != "*")
	for _, name := range allowedTools {
		allowed[name] = true
	}

	filtered := tools.NewRegistry()
	for _, t := range reg.ListTools() {
		name := t.Name()

		// Layer 1: MCP tools always allowed.
		if IsMCPTool(name) {
			filtered.Register(t)
			continue
		}

		// Layer 2: global disallowed (applies to every sub-agent).
		if AllAgentDisallowedTools[name] {
			continue
		}

		// Layer 3: custom agent extra restrictions.
		if isCustom && CustomAgentDisallowedTools[name] {
			continue
		}

		// Layer 4: async agent whitelist (with in-process teammate extension).
		if isAsync && !AsyncAgentAllowedTools[name] {
			if isInProcessTeammate {
				// In-process teammates can also use Agent (sync subagents only, validated at call site) plus
				// coordination tools.
				if name == "Agent" || InProcessTeammateAllowedTools[name] {
					filtered.Register(t)
					continue
				}
			}
			continue
		}

		// Layer 5: definition-level disallowed.
		if disallowed[name] {
			continue
		}

		// Layer 6: definition-level allowed (whitelist intersection).
		if hasWhitelist && !allowed[name] {
			continue
		}

		filtered.Register(t)
	}
	return filtered
}
