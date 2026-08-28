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

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/mcp"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/prompt"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/skills"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/teams"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/worktree"
)

// teammateArgs holds the values parsed off the CLI when this process is
// launched as a teammate worker (i.e. spawned by tmux/iTerm from
// teams.BuildTeammateCLI).
type teammateArgs struct {
	teamName   string
	memberName string
}

// parseTeammateFlags returns (args, true) when os.Args carries the
// --teammate flag, signalling teammate-worker mode. Any other value
// returns ok=false and the caller should boot the normal TUI.
//
// Format produced by teams.BuildTeammateCLI:
//
//	swiftx --teammate --team-name <t> --agent-name <n>
//
// The parsing is intentionally minimal: only the three flags this
// worker needs are recognized, and they must come as separate tokens.
func parseTeammateFlags(args []string) (teammateArgs, bool) {
	var out teammateArgs
	if len(args) == 0 || args[0] != "--teammate" {
		return out, false
	}
	i := 1
	for i < len(args) {
		switch args[i] {
		case "--team-name":
			if i+1 < len(args) {
				out.teamName = args[i+1]
				i += 2
				continue
			}
		case "--agent-name":
			if i+1 < len(args) {
				out.memberName = args[i+1]
				i += 2
				continue
			}
		}
		i++
	}
	return out, true
}

// runTeammate boots this process as a worker on an existing team. It
// loads the same config a TUI run would, builds a tools registry, then
// drops into teams.RunInProcessTeammate. The initial task is read from
// the mailbox — the lead writes it there before calling tmux/iTerm
// spawn (see teams.SpawnTeammate).
func runTeammate(args teammateArgs) error {
	if args.teamName == "" || args.memberName == "" {
		return fmt.Errorf("--teammate requires --team-name and --agent-name")
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(cfg.Providers) == 0 {
		return fmt.Errorf("no providers configured")
	}
	provider := cfg.Providers[0]

	wd, _ := os.Getwd()
	sessionID := session.NewID()

	// Worker processes get SIGINT/SIGTERM forwarded so closing the
	// pane / Ctrl-C in the tab cleanly cancels the loop and lets
	// deferred cleanup run.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// The skill catalog is injected into the conversation via the first system-reminder
	// (see ag.SkillSection below) and also attached to the LoadSkill tool for on-demand loading.
	skillCatalog := skills.LoadCatalog(wd)
	env := prompt.DetectEnvironment(wd)
	env.Model = provider.Model
	systemPrompt := prompt.BuildSystemPrompt(env)

	client, err := llm.NewClient(&provider, systemPrompt)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// The teammate process loads the team straight from config.json on disk so
	// it sees the same member roster as the lead. The team directory lives under
	// the user's home directory and is unaffected by worktree directory changes.
	teamMgr := teams.NewTeamManager()
	team := teamMgr.GetTeam(args.teamName)
	if team == nil {
		// The config hasn't been persisted yet (e.g. the lead spawns right after
		// creating the team); fall back to constructing a local copy. The mailbox
		// directory follows the same naming convention, so delivery still lines up.
		team = teams.NewTeam(args.teamName, teams.ModeInProcess)
		teamMgr.CreateTeamWith(team)
	}

	registry := buildTeammateRegistry(ctx, teammateToolOptions{
		WorkDir:       wd,
		Protocol:      provider.Protocol,
		SessionID:     sessionID,
		TeamMgr:       teamMgr,
		TeamName:      args.teamName,
		MemberName:    args.memberName,
		MCPServers:    cfg.MCPServers,
		BaseURL:       provider.BaseURL,
		ContextWindow: provider.GetContextWindow(),
	})

	member := team.AddMember(args.memberName, client, registry, provider.Protocol)
	member.AgentRef.SkillSection = buildPrintSkillSection(skillCatalog)

	// The teammate's own Agent acts as the host for the skill tools, so they can
	// only be wired in after AddMember has built the Agent. With no ForkHost,
	// skills declaring fork mode fall back to inline execution.
	registry.Register(&skills.LoadSkillTool{Catalog: skillCatalog, Host: member.AgentRef})
	registry.Register(&skills.InstallSkillTool{Catalog: skillCatalog})

	addendum := teams.BuildTeammateAddendum(args.teamName, args.memberName, nil)

	// No initial prompt argument: the lead wrote the first message to
	// the mailbox before spawn, so the loop's first idle poll picks
	// it up. Passing "" prevents RunInProcessTeammate from injecting
	// a duplicate user message.
	fmt.Fprintf(os.Stderr, "[teammate %s/%s] booted, awaiting tasks\n", args.teamName, args.memberName)
	return teams.RunInProcessTeammate(ctx, team, member, "", addendum, streamEventsToStderr())
}

// teammateToolOptions gathers the external dependencies needed to assemble the
// teammate tool set.
type teammateToolOptions struct {
	WorkDir    string
	Protocol   string
	SessionID  string
	TeamMgr    *teams.TeamManager
	TeamName   string
	MemberName string
	MCPServers []config.MCPServerConfig
	// BaseURL and ContextWindow feed the MCP loading-mode decision:
	// the former determines whether the endpoint is official, the latter
	// determines the schema-to-context ratio.
	BaseURL       string
	ContextWindow int
}

// buildTeammateRegistry assembles the teammate tool set: file and command
// tools, tool search, worktree switching, MCP extensions, plus the team
// collaboration tools (sending messages under the teammate's own name, and
// reading/writing the team-shared task board). The task board resolves to the
// same tasks.json per team name, so all teammates see the same list.
//
// Agent is deliberately absent: the call tree ends at the teammate level, and
// teammates do not spawn sub-agents of their own. TeamCreate and TeamDelete
// are absent as well — forming and disbanding teams is the lead's job.
//
// The skill tools need an Agent instance as their host, so the caller wires
// them in separately once the Agent has been built.
func buildTeammateRegistry(ctx context.Context, opts teammateToolOptions) *tools.Registry {
	registry := tools.CreateDefaultToolsWithWorkDir(opts.WorkDir).Registry

	registry.Register(&tools.ToolSearchTool{Registry: registry, Protocol: opts.Protocol})
	registry.Register(&tools.McpCallTool{Registry: registry})
	registry.Register(&tools.SyntheticOutputTool{})

	gitRoot := worktree.FindCanonicalGitRoot(opts.WorkDir)
	registry.Register(&tools.EnterWorktreeTool{SessionID: opts.SessionID, RepoRoot: gitRoot})
	registry.Register(&tools.ExitWorktreeTool{RepoRoot: gitRoot})

	registry.Register(&teams.SendMessageTool{TeamMgr: opts.TeamMgr, SenderName: opts.MemberName})
	registry.Register(&teams.TaskCreateTool{TeamMgr: opts.TeamMgr, TeamName: opts.TeamName, AgentName: opts.MemberName})
	registry.Register(&teams.TaskGetTool{TeamMgr: opts.TeamMgr, TeamName: opts.TeamName})
	registry.Register(&teams.TaskListTool{TeamMgr: opts.TeamMgr, TeamName: opts.TeamName})
	registry.Register(&teams.TaskUpdateTool{TeamMgr: opts.TeamMgr, TeamName: opts.TeamName})

	if len(opts.MCPServers) > 0 {
		mgr := mcp.NewManager()
		serverConfigs := make([]mcp.ServerConfig, 0, len(opts.MCPServers))
		for _, c := range opts.MCPServers {
			serverConfigs = append(serverConfigs, mcp.ServerConfig{
				Name:      c.Name,
				Command:   c.Command,
				Args:      c.Args,
				URL:       c.URL,
				Transport: c.Transport,
				Headers:   c.Headers,
				Env:       c.Env,
			})
		}
		mgr.LoadConfigs(serverConfigs)
		mgr.RegisterAllTools(ctx, registry)
		// All tools must be in place before measuring the schema-to-context ratio
		mcp.DecideAndApply(registry, opts.BaseURL, opts.ContextWindow)
	}

	return registry
}

// streamEventsToStderr returns a channel that forwards every agent
// event to stderr in a human-readable form. Worker processes have no
// TUI; this gives the tmux/iTerm pane something visible to show.
func streamEventsToStderr() chan<- agent.AgentEvent {
	ch := make(chan agent.AgentEvent, 32)
	go func() {
		for ev := range ch {
			switch e := ev.(type) {
			case agent.StreamText:
				fmt.Fprint(os.Stderr, e.Text)
			case agent.ToolUseEvent:
				fmt.Fprintf(os.Stderr, "\n[tool %s]\n", e.ToolName)
			case agent.ToolResultEvent:
				summary := strings.TrimSpace(e.Output)
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}
				fmt.Fprintf(os.Stderr, "[result] %s\n", summary)
			case agent.ErrorEvent:
				fmt.Fprintf(os.Stderr, "[error] %s\n", e.Message)
			case agent.LoopComplete:
				fmt.Fprintf(os.Stderr, "[turn done after %d steps]\n", e.TotalTurns)
			}
		}
	}()
	return ch
}

// _ silences the unused-import warning when conversation is referenced
// only indirectly via Member.Conv.
var _ = conversation.NewManager
