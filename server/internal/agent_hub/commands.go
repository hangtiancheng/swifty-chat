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

package agent_hub

import (
	"context"
	"fmt"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/commands"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	swiftx_session "github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/skills"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/teams"
)

func (s *Session) commandList() []CommandInfo {
	s.cmdMu.RLock()
	cmds := s.cmdRegistry.ListCommands()
	s.cmdMu.RUnlock()
	list := make([]CommandInfo, 0, len(cmds))
	for _, cmd := range cmds {
		list = append(list, CommandInfo{Name: cmd.Name, Description: cmd.Description})
	}
	return list
}

// registerSkillCommand exposes one skill as a slash command, mirroring the
// swiftx TUI wiring. Skills declared as fork-mode run inline here — the chat
// session has no sub-agent host, the same fallback LoadSkillTool applies when
// its ForkHost is nil. Idempotent: an already-taken name is left alone.
func (s *Session) registerSkillCommand(name string) {
	s.cmdMu.Lock()
	defer s.cmdMu.Unlock()
	s.registerSkillCommandLocked(name)
}

func (s *Session) registerSkillCommandLocked(name string) {
	if s.skillCatalog == nil || s.cmdRegistry == nil {
		return
	}
	if s.cmdRegistry.Find(name) != nil {
		return
	}
	meta := s.skillCatalog.Get(name)
	if meta == nil {
		return
	}
	s.cmdRegistry.Register(&commands.Command{
		Name:        name,
		Description: meta.Meta.Description + " [skill]",
		Type:        commands.TypePrompt,
		Handler: func(ctx *commands.Context) string {
			skill, err := s.skillCatalog.GetFull(name)
			if err != nil && skill == nil {
				return fmt.Sprintf("[skill error] %v", err)
			}
			body, runErr := skills.RunInline(context.Background(), skill, ctx.Args, s)
			if runErr != nil {
				return fmt.Sprintf("[skill error] %v", runErr)
			}
			s.ag.RecoveryState.RecordSkillInvocation(skill.Meta.Name, body)
			return body
		},
	})
}

// handleSlashCommand runs a slash command typed into the composer. Commands
// arrive as ordinary chat messages, so they are already visible in the
// transcript by the time they get here; only the outcome is reported back.
func (s *Session) handleSlashCommand(input string) {
	defer func() {
		if r := recover(); r != nil {
			s.emit(Event{Type: "error", Data: map[string]string{
				"message": fmt.Sprintf("Command panic: %v", r),
			}})
			s.emit(Event{Type: "command_done"})
		}
	}()

	name, args := commands.Parse(input)
	if name == "" {
		return
	}
	s.cmdMu.RLock()
	cmd := s.cmdRegistry.Find(name)
	s.cmdMu.RUnlock()
	if cmd == nil {
		s.emit(Event{Type: "error", Data: map[string]string{
			"message": fmt.Sprintf("Unknown command: /%s — type /help to see available commands", name),
		}})
		s.emit(Event{Type: "command_done"})
		return
	}
	if args == "" && cmd.ArgPrompt != "" {
		s.emit(Event{Type: "system", Data: map[string]string{"message": cmd.ArgPrompt}})
		s.emit(Event{Type: "command_done"})
		return
	}

	ctx := s.buildCommandContext(args)

	switch cmd.Type {
	case commands.TypeLocal:
		if cmd.Handler != nil {
			s.emit(Event{Type: "system", Data: map[string]string{"message": cmd.Handler(ctx)}})
		}
		s.emit(Event{Type: "command_done"})

	case commands.TypeLocalUI:
		s.handleUICommand(name, args)

	case commands.TypePrompt:
		if cmd.Handler == nil {
			s.emit(Event{Type: "command_done"})
			return
		}
		display := "/" + name
		if args != "" {
			display += " " + args
		}
		s.runTurn(display, cmd.Handler(ctx))
	}
}

func (s *Session) handleUICommand(name, args string) {
	switch name {
	case "clear":
		// Only the model's context is cleared. The transcript is the user's
		// chat history with Swiftx and stays exactly where it is; the client
		// marks the spot instead of wiping the thread.
		s.conv.Reset()
		s.ag.ClearActiveSkills()
		s.ag.SetToolFilter(teams.CoordinatorToolFilter(s.appCfg.EnableCoordinatorMode))
		s.sessionID = swiftx_session.NewID()
		s.ag.SetSessionID(s.sessionID)
		s.emit(Event{Type: "context_cleared"})

	case "compact":
		s.compactConversation()

	case "plan":
		s.ag.Checker.Mode = permissions.ModePlan
		s.ag.Checker.PlanFilePath = s.planPath()
		s.emit(Event{Type: "system", Data: map[string]string{
			"message": fmt.Sprintf("Entered Plan mode. Plan file: %s\nExplore your workspace and design an approach before making changes.", s.ag.Checker.PlanFilePath),
		}})
		if args != "" {
			s.runTurn("/plan "+args, args)
			return
		}

	case "resume":
		s.resumeSession(args)

	case "rewind":
		s.emit(Event{Type: "system", Data: map[string]string{
			"message": "Rewind is not available in chat. Use /clear to start a fresh context.",
		}})
	}
	s.emit(Event{Type: "command_done"})
}

// resumeSession points the agent's context at an earlier transcript. The chat
// thread on screen is not rewritten: what changes is only what Swiftx
// remembers, which is why the confirmation says so explicitly.
func (s *Session) resumeSession(args string) {
	sessions := swiftx_session.ListSessions(s.workDir)
	if args == "" {
		if len(sessions) == 0 {
			s.emit(Event{Type: "system", Data: map[string]string{"message": "No earlier contexts found."}})
			return
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Earlier contexts (%d):\n\n", len(sessions))
		for i, sess := range sessions {
			if i >= 20 {
				fmt.Fprintf(&sb, "  ... and %d more\n", len(sessions)-20)
				break
			}
			first := sess.FirstMessage
			if len(first) > 60 {
				first = first[:60] + "..."
			}
			fmt.Fprintf(&sb, "  %d. [%s] %s (%d msgs)\n", i+1, sess.ID, first, sess.MessageCount)
		}
		sb.WriteString("\nUsage: /resume <number> or /resume <context-id>")
		s.emit(Event{Type: "system", Data: map[string]string{"message": sb.String()}})
		return
	}

	targetID := strings.TrimSpace(args)
	var idx int
	if n, _ := fmt.Sscanf(targetID, "%d", &idx); n == 1 && idx >= 1 && idx <= len(sessions) {
		targetID = sessions[idx-1].ID
	}

	count, compacted := s.loadSessionContext(targetID)
	if count == 0 {
		s.emit(Event{Type: "error", Data: map[string]string{
			"message": fmt.Sprintf("Context '%s' not found or empty.", targetID),
		}})
		return
	}
	message := fmt.Sprintf("Swiftx is now working from context %s (%d messages). Your chat history above is unchanged.", targetID, count)
	if compacted {
		message = fmt.Sprintf("Swiftx is now working from compacted context %s (%d messages). Your chat history above is unchanged.", targetID, count)
	}
	s.emit(Event{Type: "system", Data: map[string]string{"message": message}})
}

func (s *Session) buildCommandContext(args string) *commands.Context {
	return &commands.Context{
		Args: args,
		TokenCount: func() (int, int) {
			_, _, _, usage := s.snapshot()
			return usage[0], usage[1]
		},
		PermissionMode: func() string {
			if s.ag != nil && s.ag.Checker != nil {
				return string(s.ag.Checker.Mode)
			}
			return string(permissions.ModeDefault)
		},
		ToolCount:   func() int { return len(s.registry.ListTools()) },
		SessionInfo: func() string { return fmt.Sprintf("Context: %s\nWorkspace: %s", s.sessionID, s.workDir) },
		SkillList: func() []commands.SkillInfo {
			s.cmdMu.RLock()
			defer s.cmdMu.RUnlock()
			if s.skillCatalog == nil {
				return nil
			}
			var list []commands.SkillInfo
			for _, meta := range s.skillCatalog.List() {
				list = append(list, commands.SkillInfo{Name: meta.Name, Description: meta.Description})
			}
			return list
		},
		MCPInfo: func() string {
			if s.mcpToolCount == 0 {
				return ""
			}
			return fmt.Sprintf("MCP connected (%d tools)", s.mcpToolCount)
		},
		WorkDir: s.workDir,
		Model:   s.provider.Model,
	}
}
