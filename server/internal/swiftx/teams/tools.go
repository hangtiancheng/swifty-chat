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

// SendMessageTool allows agents to send messages to named teammates.
type SendMessageTool struct {
	TeamMgr    *TeamManager
	SenderName string
}

func (t *SendMessageTool) Name() string                 { return "SendMessage" }
func (t *SendMessageTool) Category() tools.ToolCategory { return tools.CategoryCommand }
func (t *SendMessageTool) Description() string {
	return "Send a message to another named agent in the team. The recipient will see it on their next turn."
}

func (t *SendMessageTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to": map[string]any{
					"type":        "string",
					"description": "Name of the recipient agent",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Message content to send. For shutdown_request this is the reason; for plan_approval_response this is your feedback when rejecting.",
				},
				"type": map[string]any{
					"type": "string",
					"enum": []string{
						MsgText, MsgShutdownRequest, MsgShutdownResponse, MsgPlanApprovalResponse,
					},
					"description": "Message kind, defaults to 'text'. Use 'shutdown_request' to ask a teammate to wrap up (it replies with shutdown_response). Use 'plan_approval_response' to answer a teammate's plan, together with 'approve' and, when rejecting, feedback in 'content'.",
				},
				"request_id": map[string]any{
					"type":        "string",
					"description": "Required for plan_approval_response: copy the requestId from the teammate's plan approval request so it knows which plan you are answering.",
				},
				"approve": map[string]any{
					"type":        "boolean",
					"description": "Required for plan_approval_response: true to let the teammate start executing, false to send it back to revise.",
				},
			},
			"required": []string{"to", "content"},
		},
	}
}

func (t *SendMessageTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	to, _ := args["to"].(string)
	content, _ := args["content"].(string)
	msgType, _ := args["type"].(string)
	requestID, _ := args["request_id"].(string)
	approve, hasApprove := args["approve"].(bool)
	if to == "" || content == "" {
		return tools.ToolResult{Output: "Error: 'to' and 'content' are required", IsError: true}
	}

	// Structured messages use a separate channel: they carry requestId and a
	// decision flag. Embedding them in the body text would force the recipient
	// to parse natural language, regressing to "coordinating by interpreting
	// wording".
	if msgType != "" && msgType != MsgText {
		return t.sendTyped(to, msgType, requestID, content, approve, hasApprove)
	}

	// Broadcast: send the message to all other members in the sender's team.
	if to == "*" {
		for _, teamName := range t.TeamMgr.ListTeams() {
			team := t.TeamMgr.GetTeam(teamName)
			if team == nil {
				continue
			}
			if _, ok := team.Members[t.SenderName]; !ok {
				continue
			}
			count := 0
			for member := range team.Members {
				if member == t.SenderName {
					continue
				}
				team.SendMessage(t.SenderName, member, content)
				count++
			}
			return tools.ToolResult{Output: fmt.Sprintf("Message broadcast to %d teammate(s).", count)}
		}
		return tools.ToolResult{
			Output:  fmt.Sprintf("Error: cannot find team for sender '%s'", t.SenderName),
			IsError: true,
		}
	}

	// The lead is not registered as a Member (it lives in the parent
	// process and only reads from its own mailbox), so route to it by
	// finding any team the sender belongs to.
	if to == LeadName {
		for _, teamName := range t.TeamMgr.ListTeams() {
			team := t.TeamMgr.GetTeam(teamName)
			if team == nil {
				continue
			}
			if _, ok := team.Members[t.SenderName]; ok {
				team.SendMessage(t.SenderName, LeadName, content)
				return tools.ToolResult{
					Output: fmt.Sprintf("Message sent to %s.", LeadName),
				}
			}
		}
		return tools.ToolResult{
			Output:  fmt.Sprintf("Error: cannot find team for sender '%s'", t.SenderName),
			IsError: true,
		}
	}

	// Resolve the recipient name to a delivery identifier via the global name
	// registry; fall back to the original name if resolution fails.
	recipient := to
	if resolved := GetNameRegistry().Resolve(to); resolved != "" {
		recipient = resolved
	}

	// Find a registered teammate with this name, or fall back to the
	// file-based mailbox. In tmux/iTerm mode each teammate runs in a
	// separate process and only knows about itself in Members, so the
	// in-memory lookup will miss peers. The mailbox write always works
	// because all processes share the same inbox directory on disk.
	for _, teamName := range t.TeamMgr.ListTeams() {
		team := t.TeamMgr.GetTeam(teamName)
		if team == nil {
			continue
		}
		if _, ok := team.Members[recipient]; ok {
			team.SendMessage(t.SenderName, recipient, content)
			return tools.ToolResult{
				Output: fmt.Sprintf("Message sent to %s.", to),
			}
		}
		// Recipient not in Members but we belong to this team — write
		// directly to the file mailbox so external-process peers pick
		// it up on their next poll.
		if _, ok := team.Members[t.SenderName]; ok {
			team.SendMessage(t.SenderName, recipient, content)
			return tools.ToolResult{
				Output: fmt.Sprintf("Message sent to %s.", to),
			}
		}
	}

	return tools.ToolResult{
		Output:  fmt.Sprintf("Error: recipient '%s' not found in any team", to),
		IsError: true,
	}
}

// TeamCreateTool creates a new agent team.
type TeamCreateTool struct {
	TeamMgr *TeamManager
}

func (t *TeamCreateTool) Name() string                 { return "TeamCreate" }
func (t *TeamCreateTool) Category() tools.ToolCategory { return tools.CategoryCommand }
func (t *TeamCreateTool) Description() string {
	return `Create a new team for coordinating multiple agents.

## When to Use

Use this tool proactively whenever:
- The user explicitly asks to use a team, swarm, or group of agents
- The user mentions wanting agents to work together, coordinate, or collaborate
- A task requires sequential or parallel collaboration between multiple agents

When in doubt about whether a task warrants a team, prefer spawning a team.

## Team Workflow

1. **Create a team** with TeamCreate
2. **Spawn teammates** using the Agent tool with team_name and name parameters — this is REQUIRED to create long-running team members
3. Teammates work independently and communicate via **SendMessage**
4. When a teammate finishes, it sends its result to "lead" via SendMessage, then goes idle
5. The lead collects and synthesizes all teammate results

## CRITICAL: Spawning Teammates

To add a member to a team, you MUST pass both team_name and name to the Agent tool:
` + "```" + `
Agent({
  "team_name": "<team name from step 1>",
  "name": "<member name, e.g. reviewer>",
  "prompt": "...",
  "description": "..."
})
` + "```" + `
Without team_name, the agent runs as a one-shot sub-agent that blocks and returns inline — it will NOT be a team member.

## Teammate Idle State

Teammates go idle after every turn — this is completely normal. A teammate going idle after sending a message does NOT mean they are done or unavailable. Sending a message to an idle teammate wakes them up.

## Communication

- Use SendMessage to talk to teammates by name
- Messages from teammates arrive as system reminders at the start of each turn
- Messages are delivered automatically — you do NOT need to manually check your inbox`
}

func (t *TeamCreateTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team_name": map[string]any{
					"type":        "string",
					"description": "Name for the team",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "What this team will work on",
				},
			},
			"required": []string{"team_name"},
		},
	}
}

func (t *TeamCreateTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	name, _ := args["team_name"].(string)
	if name == "" {
		return tools.ToolResult{Output: "Error: team_name is required", IsError: true}
	}

	// Deduplicate: if name exists, append suffix
	baseName := name
	for i := 2; t.TeamMgr.GetTeam(name) != nil; i++ {
		name = fmt.Sprintf("%s-%d", baseName, i)
	}

	mode := detectBackend()
	desc, _ := args["description"].(string)
	team := t.TeamMgr.CreateTeamFull(name, mode, LeadName, desc)
	return tools.ToolResult{
		Output: fmt.Sprintf("Team \"%s\" created (mode: %s). Use Agent tool with team_name=\"%s\" to add teammates.\nDescription: %s",
			team.Name, team.Mode, team.Name, desc),
	}
}

// TeamDeleteTool deletes an agent team and stops all members.
type TeamDeleteTool struct {
	TeamMgr *TeamManager
}

func (t *TeamDeleteTool) Name() string { return "TeamDelete" }

func (t *TeamDeleteTool) Category() tools.ToolCategory { return tools.CategoryCommand }
func (t *TeamDeleteTool) Description() string {
	return "Delete a team, stopping all its members."
}

func (t *TeamDeleteTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team_name": map[string]any{
					"type":        "string",
					"description": "Name of the team to delete",
				},
			},
			"required": []string{"team_name"},
		},
	}
}

func (t *TeamDeleteTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	name, _ := args["team_name"].(string)
	if name == "" {
		return tools.ToolResult{Output: "Error: team_name is required", IsError: true}
	}

	team := t.TeamMgr.GetTeam(name)
	if team == nil {
		return tools.ToolResult{
			Output:  fmt.Sprintf("Error: team '%s' not found", name),
			IsError: true,
		}
	}

	memberCount := len(team.Members)
	var memberNames []string
	for n := range team.Members {
		memberNames = append(memberNames, n)
	}

	t.TeamMgr.DeleteTeam(name)
	return tools.ToolResult{
		Output: fmt.Sprintf("Team \"%s\" deleted. Stopped %d member(s): %s", name, memberCount, strings.Join(memberNames, ", ")),
	}
}

// sendTyped delivers a structured message. It uses the same mailbox as plain
// text; the only difference is that the message carries Type / RequestID /
// Approve fields, allowing the recipient to decide based on fields rather than
// parsing wording.
func (t *SendMessageTool) sendTyped(to, msgType, requestID, content string, approve, hasApprove bool) tools.ToolResult {
	var msg FileMailMessage
	switch msgType {
	case MsgShutdownRequest:
		msg = NewShutdownRequest(t.SenderName, content)
	case MsgShutdownResponse:
		if !hasApprove {
			return tools.ToolResult{Output: "Error: shutdown_response requires 'approve'", IsError: true}
		}
		msg = NewShutdownResponse(t.SenderName, requestID, approve, content)
	case MsgPlanApprovalResponse:
		if requestID == "" || !hasApprove {
			return tools.ToolResult{
				Output:  "Error: plan_approval_response requires both 'request_id' and 'approve'",
				IsError: true,
			}
		}
		msg = NewPlanApprovalResponse(t.SenderName, requestID, approve, content)
	default:
		return tools.ToolResult{Output: "Error: unsupported message type " + msgType, IsError: true}
	}

	team := t.senderTeam()
	if team == nil {
		return tools.ToolResult{
			Output:  fmt.Sprintf("Error: cannot find team for sender '%s'", t.SenderName),
			IsError: true,
		}
	}
	if err := team.MailBox.Send(to, msg); err != nil {
		return tools.ToolResult{Output: "Error sending message: " + err.Error(), IsError: true}
	}
	return tools.ToolResult{Output: fmt.Sprintf("%s sent to %s.", msgType, to)}
}

// senderTeam finds the team the sender belongs to. The Lead itself is not in
// Members; when it sends a message, use the only team under its name.
func (t *SendMessageTool) senderTeam() *Team {
	names := t.TeamMgr.ListTeams()
	for _, name := range names {
		team := t.TeamMgr.GetTeam(name)
		if team == nil {
			continue
		}
		if _, ok := team.Members[t.SenderName]; ok {
			return team
		}
	}
	if t.SenderName == LeadName && len(names) > 0 {
		return t.TeamMgr.GetTeam(names[0])
	}
	return nil
}
