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

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// TeammateSpawnConfig collects every parameter SpawnTeammate needs.
// Fields apply across backends:
// Team / MemberName / Task / Addendum: always used.
// Client / Registry / Protocol: only in-process; external backends
// load these themselves from the spawned process's own config.
// Workdir: optional working directory override. When non-empty,
// in-process members get their Agent.WorkDir pointed there and
// tmux/iTerm spawns cd into that path. Used for worktree
// isolation so concurrent teammates don't fight over files.
type TeammateSpawnConfig struct {
	Team       *Team
	MemberName string
	Task       string
	Addendum   string

	Client   llm.Client
	Registry *tools.Registry
	Protocol string

	Workdir string

	// Checker is the teammate's permission checker. When the Lead marks
	// plan_mode_required during dispatch, this is a ModePlan checker — the
	// teammate can only read, not modify, until the plan is approved.
	Checker *permissions.Checker
}

// SpawnResult carries the per-backend handle returned by SpawnTeammate. In-process spawns get an
// event channel; tmux/iTerm spawns get a pane handle that's also stored on Member.PaneID for later
// teardown.
type SpawnResult struct {
	Mode    TeamMode
	EventCh <-chan agent.AgentEvent // in-process only
	PaneID  string                  // tmux/iTerm only
}

// SpawnTeammate creates a new team member and launches it under the team's currently selected
// backend (Team.Mode). It is the single entry point used by the Agent tool's team_name code path;
// the dispatch below / .
//
// For external backends, the teammate's initial task is delivered via the mailbox before the new
// process boots, so the teammate sees the task on its first idle poll.
func SpawnTeammate(ctx context.Context, cfg TeammateSpawnConfig) (*SpawnResult, error) {
	if cfg.Team == nil {
		return nil, fmt.Errorf("SpawnTeammate: team is required")
	}
	if cfg.MemberName == "" {
		return nil, fmt.Errorf("SpawnTeammate: member name is required")
	}

	// Register the member name in the global name registry so SendMessage can
	// resolve and deliver by name.
	GetNameRegistry().Register(cfg.MemberName, cfg.MemberName)

	switch cfg.Team.Mode {
	case ModeInProcess:
		ch := StartInProcessMember(
			ctx,
			cfg.Team,
			cfg.MemberName,
			cfg.Client,
			cfg.Registry,
			cfg.Protocol,
			cfg.Task,
			cfg.Addendum,
		)
		// Workdir applies to the just-registered member's Agent so every file/Bash tool resolves relative
		// to the isolated path.
		if m, ok := cfg.Team.Members[cfg.MemberName]; ok && m.AgentRef != nil {
			if cfg.Workdir != "" {
				m.AgentRef.WorkDir = cfg.Workdir
			}
			m.AgentRef.Checker = cfg.Checker
		}
		return &SpawnResult{Mode: ModeInProcess, EventCh: ch}, nil

	case ModeTmux:
		// External processes pick up their task via the mailbox. Drop the initial assignment in before
		// spawn so the new process sees work on its first poll.
		if cfg.Task != "" {
			_ = cfg.Team.MailBox.Send(cfg.MemberName, FileMailMessage{
				From: LeadName,
				Text: cfg.Task,
			})
		}
		cliCommand, err := BuildTeammateCLI(cfg.Team.Name, cfg.MemberName, cfg.Workdir)
		if err != nil {
			return nil, err
		}
		paneID, err := spawnTmuxTeammate(cfg.Team.Name, cfg.MemberName, cliCommand)
		if err != nil {
			return nil, err
		}
		cfg.Team.recordExternalMember(cfg.MemberName, paneID)
		return &SpawnResult{Mode: ModeTmux, PaneID: paneID}, nil

	case ModeITerm:
		if cfg.Task != "" {
			_ = cfg.Team.MailBox.Send(cfg.MemberName, FileMailMessage{
				From: LeadName,
				Text: cfg.Task,
			})
		}
		cliCommand, err := BuildTeammateCLI(cfg.Team.Name, cfg.MemberName, cfg.Workdir)
		if err != nil {
			return nil, err
		}
		tabID, err := spawnITermTeammate(cfg.Team.Name, cfg.MemberName, cliCommand)
		if err != nil {
			return nil, err
		}
		cfg.Team.recordExternalMember(cfg.MemberName, tabID)
		return &SpawnResult{Mode: ModeITerm, PaneID: tabID}, nil
	}

	return nil, fmt.Errorf("unknown team mode: %s", cfg.Team.Mode)
}

// recordExternalMember registers a tmux/iTerm-spawned teammate in the in-memory Members map so
// StopMember can later target it for teardown. External teammates don't have an AgentRef on this
// side — their LLM lives in the spawned process — so the entry is just a name+handle stub used by
// the lead's coordination tools.
func (t *Team) recordExternalMember(name, paneID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Members[name] = &Member{
		Name:   name,
		Active: true,
		PaneID: paneID,
	}
}

// BuildTeammateCLI returns the shell command that, when run in a new terminal pane/tab, boots this
// swiftx binary in teammate mode for the given team/member. The workdir argument controls where
// the spawned process runs; passing "" falls back to the lead's current directory so the mailbox
// path resolves identically. Worktree isolation is the expected use of a non-empty workdir.
//
// Output format matches the cmd line parsed by cmd/swiftx/main.go's teammate-mode branch.
func BuildTeammateCLI(teamName, memberName, workdir string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate swiftx binary: %w", err)
	}
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	return fmt.Sprintf(
		"cd %s && %s --teammate --team-name %s --agent-name %s",
		shellQuote(workdir),
		shellQuote(exe),
		shellQuote(teamName),
		shellQuote(memberName),
	), nil
}

// shellQuote wraps a value for safe inclusion in a /bin/sh -c argument. Single-quote escaping is
// used because tmux send-keys and osascript `write text` both interpret the string through a shell.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
