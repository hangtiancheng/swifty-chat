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
	"os"
	"strings"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/plan_file"
)

// LeadName is the conventional sender/recipient identifier used by the coordinator side. Teammates
// send idle notifications here and read the lead's task assignments from messages with From ==
// LeadName.
const LeadName = "lead"

// ShutdownPrefix marks a mailbox message as a request to terminate the teammate. The lead writes
// one of these to wind down a member cleanly; the runner sees it during idle polling and returns
// from the loop.
const ShutdownPrefix = "[shutdown]"

// IdlePollInterval is how often an idle teammate scans its inbox for new work.
const IdlePollInterval = 500 * time.Millisecond

// IsShutdownRequest reports whether a mailbox message asks the teammate to exit by matching the
// shutdown prefix.

// CreateIdleNotification builds the message a teammate sends to the lead after finishing a turn.
// The lead routes work by reading these.
func CreateIdleNotification(memberName, reason string) FileMailMessage {
	return NewFileMailMessage(memberName, fmt.Sprintf("[idle] %s (reason: %s)", memberName, reason))
}

// RunInProcessTeammate drives a teammate's main loop in the current process. It blocks until ctx is
// cancelled or a shutdown request lands in the inbox. Each iteration:
//
// 1. waitForNextPromptOrShutdown — fold any pending mailbox messages into a user prompt (or return
// on shutdown / cancellation). 2. runAgent — call agent.Run on the shared conversation; forward
// events through eventOut. The channel closing signals turn-end. 3. sendIdleNotification — drop an
// idle marker into the lead's inbox so it can dispatch the next task.
//
// This The initial prompt jump-starts the first iteration; subsequent iterations get their prompt
// from the mailbox.
func RunInProcessTeammate(
	ctx context.Context,
	team *Team,
	member *Member,
	initialPrompt string,
	addendum string,
	eventOut chan<- agent.AgentEvent,
) error {
	if addendum != "" {
		member.Conv.AddSystemReminder(addendum)
	}

	nextPrompt := initialPrompt
	idleReason := "available"

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Fold any messages that landed in the inbox before this turn into the conversation as a system
		// reminder so the model sees them as inbound notifications, not user instructions.
		if reminder := InjectPendingMessages(team, member.Name); reminder != "" {
			member.Conv.AddSystemReminder(reminder)
		}

		if nextPrompt != "" {
			member.Conv.AddUserMessage(nextPrompt)
		}
		nextPrompt = ""

		ch := member.AgentRef.Run(ctx, member.Conv)
		for ev := range ch {
			// Update progress tracking
			if member.Progress != nil {
				switch e := ev.(type) {
				case agent.ToolUseEvent:
					member.Progress.RecordToolUse(e.ToolName, e.Args)
				case agent.UsageEvent:
					member.Progress.RecordTokens(int64(e.InputTokens), int64(e.OutputTokens))
				}
			}
			if eventOut != nil {
				select {
				case eventOut <- ev:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			if e, ok := ev.(agent.ErrorEvent); ok && e.Message != "" {
				idleReason = "failed"
			}
		}

		if member.Progress != nil {
			if idleReason == "failed" {
				member.Progress.SetStatus("failed")
			} else {
				member.Progress.SetStatus("idle")
			}
		}

		// Plan-mode teammate: a completed turn means it called ExitPlanMode and
		// the plan has been written to disk. Submit the plan to the Lead for
		// approval; only lift the read-only restriction once approved.
		if planModeActive(member) {
			approved, feedback, err := requestPlanApproval(ctx, team, member)
			if err != nil {
				return err
			}
			if approved {
				// After approval, switch back to normal permissions so the teammate can modify files.
				member.AgentRef.Checker.Mode = permissions.ModeDefault
				nextPrompt = "The Lead has approved your plan. Begin execution now according to the plan."
			} else {
				// On rejection, stay in plan mode and rewrite the plan with the feedback.
				nextPrompt = "The Lead rejected your plan. Revision feedback: " + feedback + "\nPlease revise the plan accordingly and resubmit."
			}
			continue
		}

		// Notify the lead that this teammate finished its turn so the lead can decide whether to feed it
		// more work.
		_ = team.MailBox.Send(LeadName, CreateIdleNotification(member.Name, idleReason))
		idleReason = "available"

		// Idle poll. Sleep IdlePollInterval, then drain the inbox. Stop on shutdown messages; otherwise
		// build the next prompt and loop back.
		prompt, shutdown, err := waitForNextPromptOrShutdown(ctx, team, member.Name)
		if err != nil {
			return err
		}
		if shutdown != nil {
			// Before wrapping up, give the Lead an explicit acknowledgment so it
			// knows the pane can be reclaimed. Teammates always agree here: they
			// are already in idle polling with no work in progress. The real
			// refusal scenario is being interrupted mid-task, in which case the
			// teammate would never reach this polling point.
			if shutdown.Type == MsgShutdownRequest {
				_ = team.MailBox.Send(LeadName,
					NewShutdownResponse(member.Name, shutdown.RequestID, true, "acknowledged, shutting down"))
			}
			return nil
		}
		nextPrompt = prompt
	}
}

// waitForNextPromptOrShutdown blocks until the inbox has at least one message, then turns the
// unread batch into the next user prompt. If any message is a shutdown request, the function
// returns shutdown=true without building a prompt.
// planModeActive reports whether a teammate is in plan mode. Only teammates
// marked with planModeRequired by the Lead enter this mode; regular teammates
// work directly.
func planModeActive(member *Member) bool {
	return member.AgentRef != nil &&
		member.AgentRef.Checker != nil &&
		member.AgentRef.Checker.Mode == permissions.ModePlan
}

// requestPlanApproval sends the teammate's completed plan to the Lead and
// blocks until a decision is received.
//
// The teammate holds read-only permissions at this point, so waiting
// indefinitely causes no harm; no timeout is set here. Rather than timing out
// and presumptuously starting to modify files, it is better to wait and let
// the user drive progress from the Lead side.
func requestPlanApproval(ctx context.Context, team *Team, member *Member) (bool, string, error) {
	plan := readPlanForReview(member)
	req := NewPlanApprovalRequest(member.Name, plan)
	if err := team.MailBox.Send(LeadName, req); err != nil {
		return false, "", err
	}
	if member.Progress != nil {
		member.Progress.SetStatus("awaiting plan approval")
	}

	for {
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		case <-time.After(IdlePollInterval):
		}

		msgs, err := team.MailBox.ReadUnread(member.Name)
		if err != nil {
			return false, "", err
		}
		for _, m := range msgs {
			// Only accept the response matching this request; leave other
			// messages for the next turn.
			if m.Type == MsgPlanApprovalResponse && m.RequestID == req.RequestID {
				_ = team.MailBox.MarkAllRead(member.Name)
				return m.Approved(), m.Text, nil
			}
		}
	}
}

// readPlanForReview reads the full plan written by the teammate for the Lead to review.
func readPlanForReview(member *Member) string {
	workDir := ""
	if member.AgentRef != nil {
		workDir = member.AgentRef.WorkDir
	}
	path := plan_file.GetOrCreatePlanPath(workDir)
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return "(plan file is empty; the teammate may not have written the plan as required)"
	}
	return string(data)
}

func waitForNextPromptOrShutdown(ctx context.Context, team *Team, memberName string) (string, *FileMailMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(IdlePollInterval):
		}

		msgs, err := team.MailBox.ReadUnread(memberName)
		if err != nil {
			return "", nil, err
		}
		if len(msgs) == 0 {
			continue
		}

		var shutdown *FileMailMessage
		var keep []FileMailMessage
		for i, m := range msgs {
			if IsShutdownRequest(m) {
				shutdown = &msgs[i]
				continue
			}
			keep = append(keep, m)
		}
		_ = team.MailBox.MarkAllRead(memberName)

		if shutdown != nil {
			return "", shutdown, nil
		}
		return formatInboundAsPrompt(keep), nil, nil
	}
}

// DrainLeadMailbox reads every unread notification in every team's lead inbox and returns them as
// system-reminder strings (one per team). The lead's main loop installs this in
// Agent.NotificationFn so teammate idle notifications surface to the model at the top of each turn.
func DrainLeadMailbox(mgr *TeamManager) []string {
	if mgr == nil {
		return nil
	}
	var notes []string
	for _, name := range mgr.ListTeams() {
		team := mgr.GetTeam(name)
		if team == nil {
			continue
		}
		msgs, err := team.MailBox.ReadUnread(LeadName)
		if err != nil || len(msgs) == 0 {
			continue
		}
		var sb strings.Builder
		sb.WriteString("<team-notification team=\"")
		sb.WriteString(name)
		sb.WriteString("\">\n")
		for _, m := range msgs {
			sb.WriteString("from=")
			sb.WriteString(m.From)
			sb.WriteString(": ")
			sb.WriteString(m.Text)
			sb.WriteString("\n")
		}
		sb.WriteString("</team-notification>")
		notes = append(notes, sb.String())
		_ = team.MailBox.MarkAllRead(LeadName)
	}
	return notes
}

// formatInboundAsPrompt turns an unread batch into a single user prompt. Each message is tagged
// with its sender so the teammate can route a reply. Matches formatAsTeammateMessage in ,
// simplified to plain text instead of XML.
func formatInboundAsPrompt(msgs []FileMailMessage) string {
	if len(msgs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("You have new messages from your team:\n\n")
	for _, m := range msgs {
		fmt.Fprintf(&sb, "From %s: %s\n\n", m.From, m.Text)
	}
	return sb.String()
}

// _ silences the unused-import warning when conversation is referenced only via Member.Conv
// methods.
var _ = conversation.NewManager
