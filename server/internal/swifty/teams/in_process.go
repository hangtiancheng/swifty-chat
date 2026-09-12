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
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/agent"
)

// StartInProcessMember registers a teammate on the team and launches its long-running main loop in
// a background goroutine. The returned channel forwards every AgentEvent emitted across all turns;
// it closes when the loop exits (ctx cancellation or shutdown request in the inbox).
//
// The lifecycle of the goroutine is bound to ctx: the caller cancels ctx to stop the teammate. Each
// pass through the loop calls RunInProcessTeammate, which handles waiting, agent execution, and
// idle notification.
//
// All member configuration (agent client, permission checker, workdir,
// metadata) is applied inside AddMember before the goroutine starts, so the
// first turn already sees the final setup.
func StartInProcessMember(ctx context.Context, cfg TeammateSpawnConfig) <-chan agent.AgentEvent {
	team := cfg.Team
	member := team.AddMember(cfg.MemberName, MemberInit{
		Client:       cfg.Client,
		Registry:     cfg.Registry,
		Protocol:     cfg.Protocol,
		Checker:      cfg.Checker,
		AgentType:    cfg.AgentType,
		Model:        cfg.Model,
		WorktreePath: cfg.Workdir,
	})

	team.mu.Lock()
	memberCtx, cancel := context.WithCancel(ctx)
	member.Active = true
	member.Cancel = cancel
	team.mu.Unlock()

	eventCh := make(chan agent.AgentEvent, 32)
	go func() {
		defer close(eventCh)
		defer func() {
			// Persist conversation transcript when teammate exits, for debugging
			if member.Conv != nil {
				_, _ = SaveTranscript(team.Name, cfg.MemberName, member.Conv)
			}
			team.mu.Lock()
			member.Active = false
			team.mu.Unlock()
		}()
		_ = RunInProcessTeammate(memberCtx, team, member, cfg.Task, cfg.Addendum, eventCh)
	}()
	return eventCh
}

// BuildTeammateAddendum creates the system-reminder text injected at the top of every teammate's
// conversation. It tells the model its identity, who else is on the team, and how to send messages.
func BuildTeammateAddendum(teamName, memberName string, otherMembers []string) string {
	var sb strings.Builder
	sb.WriteString("You are a member of team \"")
	sb.WriteString(teamName)
	sb.WriteString("\". Your name is \"")
	sb.WriteString(memberName)
	sb.WriteString("\".\n\n")
	sb.WriteString("The lead is reachable as \"" + LeadName + "\". Deliver your final result to the lead with SendMessage(to=\"" + LeadName + "\", content=...) — the idle notification alone only signals completion, it does not carry your output.\n")
	if len(otherMembers) > 0 {
		sb.WriteString("Other team members: " + strings.Join(otherMembers, ", ") + "\n")
	}
	sb.WriteString("\nYou can communicate with the lead and teammates using the SendMessage tool.\n")
	sb.WriteString("Messages from the team arrive as system reminders at the start of each turn.\n")
	sb.WriteString("When you finish your current task, send your final result to \"" + LeadName + "\" via SendMessage, then stop calling tools — an idle notification will be sent to the lead automatically.\n")
	return sb.String()
}

// InjectPendingMessages returns any unread mailbox messages formatted as a system-reminder string
// and marks them read. It is called at the top of every teammate turn by RunInProcessTeammate; the
// empty return means no new mail.
func InjectPendingMessages(team *Team, memberName string) string {
	msgs, err := team.MailBox.ReadUnread(memberName)
	if err != nil || len(msgs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("You have new messages:\n\n")
	for _, msg := range msgs {
		sb.WriteString("From ")
		sb.WriteString(msg.From)
		sb.WriteString(": ")
		sb.WriteString(msg.Text)
		sb.WriteString("\n\n")
	}

	_ = team.MailBox.MarkAllRead(memberName)
	return sb.String()
}
