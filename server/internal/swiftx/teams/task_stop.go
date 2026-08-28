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
	"fmt"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// TaskStopTool stops a running teammate.
// The Coordinator uses it to cut losses when a teammate was sent in the wrong
// direction, without waiting for it to finish the incorrect work.
//
// It connects to TeamManager rather than the background task board: in
// coordinator mode the Lead dispatches teammates via the Agent tool with
// team_name; the Team holds their Cancel functions, and they do not exist in
// the background task board.
type TaskStopTool struct {
	TeamMgr *TeamManager
}

func (t *TaskStopTool) Name() string                 { return "TaskStop" }
func (t *TaskStopTool) Category() tools.ToolCategory { return tools.CategoryCommand }

func (t *TaskStopTool) Description() string {
	return "Stop a running teammate. Pass the teammate name as it appears in the from= field of a team-notification. " +
		"Use this when you sent a teammate in the wrong direction — for example when the user " +
		"changes requirements after you launched it."
}

func (t *TaskStopTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"teammate": map[string]any{
					"type":        "string",
					"description": "Name of the teammate to stop, exactly as it appears in the from= field of a team-notification",
				},
			},
			"required": []string{"teammate"},
		},
	}
}

func (t *TaskStopTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	name, _ := args["teammate"].(string)
	if name == "" {
		return tools.ToolResult{Output: "Error: teammate is required", IsError: true}
	}
	if t.TeamMgr == nil {
		return tools.ToolResult{Output: "Error: team manager unavailable", IsError: true}
	}

	// Teammate names may be duplicated across teams; only stop in the team
	// that actually contains this member to avoid killing a same-named
	// teammate in another team.
	for _, teamName := range t.TeamMgr.ListTeams() {
		team := t.TeamMgr.GetTeam(teamName)
		if team == nil {
			continue
		}
		member, ok := team.Members[name]
		if !ok {
			continue
		}
		if !member.Active {
			return tools.ToolResult{
				Output: fmt.Sprintf("Teammate '%s' in team '%s' is not running, nothing to stop", name, teamName),
			}
		}
		team.StopMember(name)
		return tools.ToolResult{
			Output: fmt.Sprintf("Teammate '%s' in team '%s' stopped.", name, teamName),
		}
	}

	return tools.ToolResult{
		Output:  fmt.Sprintf("Error: teammate '%s' not found. Known teammates: %s", name, t.knownMembers()),
		IsError: true,
	}
}

// knownMembers lists all current teammate names for the model, preventing it
// from retrying endlessly with a misremembered name.
func (t *TaskStopTool) knownMembers() string {
	var names []string
	for _, teamName := range t.TeamMgr.ListTeams() {
		team := t.TeamMgr.GetTeam(teamName)
		if team == nil {
			continue
		}
		for memberName := range team.Members {
			names = append(names, memberName)
		}
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}
