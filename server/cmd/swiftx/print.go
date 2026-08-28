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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/file_history"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/hooks"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/mcp"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/prompt"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/skills"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/subagent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/teams"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/todo"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/worktree"
)

type printResult struct {
	Type       string         `json:"type"`
	Result     string         `json:"result"`
	DurationMs int64          `json:"duration_ms"`
	NumTurns   int            `json:"num_turns"`
	ToolCalls  []toolCallInfo `json:"tool_calls"`
	Usage      usageInfo      `json:"usage"`
	StopReason string         `json:"stop_reason"`
}

type toolCallInfo struct {
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

type usageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// parsePrintFlags parses -p/--print mode arguments from the command line.
// Returns prompt, outputFormat, ok.
func parsePrintFlags(args []string) (string, string, bool) {
	isPrint := false
	outputFormat := "text"
	var prompt string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--print":
			isPrint = true
		case "--output-format":
			if i+1 < len(args) {
				outputFormat = args[i+1]
				i++
			}
		default:
			if isPrint && prompt == "" && !strings.HasPrefix(args[i], "-") {
				prompt = args[i]
			}
		}
	}

	return prompt, outputFormat, isPrint
}

func runPrint(userPrompt string, cfg *config.AppConfig, hookConfigs []hooks.Hook, outputFormat string) error {
	p := &cfg.Providers[0]
	wd, _ := os.Getwd()

	defaultTools := tools.CreateDefaultTools()
	registry := defaultTools.Registry

	skillCatalog := skills.LoadCatalog(wd)
	instructionsContent := loadPrintInstructions(wd)
	memoryContent := memory.LoadAutoMemoryPrompt(wd)
	skillSection := buildPrintSkillSection(skillCatalog)

	env := prompt.DetectEnvironment(wd)
	env.Model = p.Model
	// The system prompt contains only project-agnostic product definitions;
	// instructions, auto-memory, and the skill catalog are project-scoped and
	// injected into the conversation via conversation.InjectLongTermMemory as a system-reminder.
	systemPrompt := prompt.BuildSystemPrompt(env)

	client, err := llm.NewClient(p, systemPrompt)
	if err != nil {
		return err
	}

	conv := conversation.NewManager()
	sessionID := session.NewID()
	fh := file_history.New(wd, sessionID)
	defaultTools.EditFile.FileHistory = fh
	defaultTools.WriteFile.FileHistory = fh

	llm.ResolveContextWindow(context.Background(), p)

	// Register tools
	taskMgr := subagent.NewTaskManager()
	store := todo.NewStore(wd, sessionID)
	todoList := todo.NewTaskList(store)
	loader := subagent.NewAgentLoader(wd)
	loader.LoadAll()

	registry.Register(&todo.TaskCreateTool{List: todoList})
	registry.Register(&todo.TaskGetTool{List: todoList})
	registry.Register(&todo.TaskListTool{List: todoList})
	registry.Register(&todo.TaskUpdateTool{List: todoList})
	registry.Register(&tools.ToolSearchTool{Registry: registry, Protocol: p.Protocol})
	registry.Register(&tools.McpCallTool{Registry: registry})

	// Team tools are also available in -p mode; the lead can assemble a team and delegate work in a single non-interactive run
	teamMgr := teams.NewTeamManager()
	registry.Register(&teams.TeamCreateTool{TeamMgr: teamMgr})
	registry.Register(&teams.TeamDeleteTool{TeamMgr: teamMgr})
	registry.Register(&teams.SendMessageTool{TeamMgr: teamMgr, SenderName: "lead"})
	registry.Register(&teams.TaskStopTool{TeamMgr: teamMgr})
	registry.Register(&tools.SyntheticOutputTool{})

	// Connect MCP. Placed after all built-in tools are registered: the MCP
	// loading mode depends on the ratio of total schema size to context
	// window, which can only be measured accurately once all tools are in place.
	if len(cfg.MCPServers) > 0 {
		mcpMgr := mcp.NewManager()
		serverConfigs := make([]mcp.ServerConfig, 0, len(cfg.MCPServers))
		for _, c := range cfg.MCPServers {
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
		mcpMgr.LoadConfigs(serverConfigs)
		for _, e := range mcpMgr.RegisterAllTools(context.Background(), registry) {
			fmt.Fprintf(os.Stderr, "MCP warning: %s\n", e)
		}
		mcp.DecideAndApply(registry, p.BaseURL, p.GetContextWindow())
		defer mcpMgr.Shutdown()
	}

	subProgressCh := make(chan subagent.SubAgentProgress, 32)
	registry.Register(&subagent.AgentTool{
		Client:        client,
		ModelResolver: llm.NewModelResolver(*p),
		Registry:      registry,
		Protocol:      p.Protocol,
		TaskMgr:       taskMgr,
		ProgressCh:    subProgressCh,
		Loader:        loader,
		Conversation:  conv,
		TeamMgr:       teamMgr,
		ForkDisabled:  !cfg.ForkEnabled(),
	})

	ag := agent.New(client, registry, p.Protocol)
	ag.ContextWindow = p.GetContextWindow()
	ag.MaxOutputTokens = p.GetMaxOutputTokens()
	ag.Instructions = instructionsContent
	ag.MemoryContent = memoryContent
	ag.SkillSection = skillSection
	ag.FileHistory = fh
	ag.SetSessionID(sessionID)

	// Print mode automatically bypasses all permission checks
	sandboxAllow := []string{memory.GetAutoMemPath(wd)}
	if userMem := memory.GetUserAutoMemPath(); userMem != "" {
		sandboxAllow = append(sandboxAllow, userMem)
	}
	ag.Checker = permissions.NewChecker(
		permissions.NewPathSandbox(wd, sandboxAllow...),
		permissions.NewRuleEngine(wd),
		permissions.ModeBypass,
	)

	if len(hookConfigs) > 0 {
		eng := hooks.NewEngine()
		eng.LoadHooks(hookConfigs)
		ag.Hooks = eng
	}

	// Completed teammate responses land in the lead's mailbox; drained each turn as system-reminders for the lead
	ag.NotificationFn = func() []string {
		return teams.DrainLeadMailbox(teamMgr)
	}
	ag.ToolNameFilter = teams.CoordinatorToolFilter(cfg.EnableCoordinatorMode)
	ag.CoordinatorActiveFn = teams.CoordinatorActiveFn(cfg.EnableCoordinatorMode)

	if at, ok := registry.Get("Agent").(*subagent.AgentTool); ok {
		at.ParentChecker = ag.Checker
	}

	gitRoot := worktree.FindCanonicalGitRoot(wd)
	registry.Register(&tools.EnterWorktreeTool{SessionID: sessionID, RepoRoot: gitRoot})
	registry.Register(&tools.ExitWorktreeTool{RepoRoot: gitRoot})

	// Execute
	conv.AddUserMessage(userPrompt)
	ctx := context.Background()
	ch := ag.Run(ctx, conv)

	start := time.Now()
	var textBuf strings.Builder
	var totalInput, totalOutput int
	var toolCalls []toolCallInfo
	isJSON := outputFormat == "stream-json"

	for ev := range ch {
		switch e := ev.(type) {
		case agent.StreamText:
			textBuf.WriteString(e.Text)
			if isJSON {
				emitJSON(map[string]any{"type": "assistant", "text": e.Text})
			}

		case agent.ThinkingText:
			if isJSON {
				emitJSON(map[string]any{"type": "thinking", "text": e.Text})
			}

		case agent.ToolUseEvent:
			toolCalls = append(toolCalls, toolCallInfo{Name: e.ToolName})
			if isJSON {
				emitJSON(map[string]any{
					"type":      "tool_use",
					"tool_name": e.ToolName,
					"tool_id":   e.ToolID,
					"args":      e.Args,
				})
			}

		case agent.ToolResultEvent:
			if len(toolCalls) > 0 {
				toolCalls[len(toolCalls)-1].IsError = e.IsError
			}
			if isJSON {
				emitJSON(map[string]any{
					"type":      "tool_result",
					"tool_name": e.ToolName,
					"tool_id":   e.ToolID,
					"output":    e.Output,
					"is_error":  e.IsError,
					"elapsed":   e.Elapsed.Seconds(),
				})
			}

		case agent.UsageEvent:
			totalInput = e.InputTokens
			totalOutput = e.OutputTokens
			if isJSON {
				emitJSON(map[string]any{
					"type":          "usage",
					"input_tokens":  e.InputTokens,
					"output_tokens": e.OutputTokens,
				})
			}

		case agent.TurnComplete:
			if isJSON {
				emitJSON(map[string]any{"type": "turn_complete", "turn": e.Turn})
			}

		case agent.LoopComplete:
			elapsed := time.Since(start)
			if isJSON {
				emitJSON(printResult{
					Type:       "result",
					Result:     textBuf.String(),
					DurationMs: elapsed.Milliseconds(),
					NumTurns:   e.TotalTurns,
					ToolCalls:  toolCalls,
					Usage:      usageInfo{InputTokens: totalInput, OutputTokens: totalOutput},
					StopReason: "end_turn",
				})
			} else {
				fmt.Print(textBuf.String())
			}
			return nil

		case agent.ErrorEvent:
			if isJSON {
				emitJSON(map[string]any{"type": "error", "message": e.Message})
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", e.Message)
			}

		case agent.CompactEvent:
			if isJSON {
				emitJSON(map[string]any{"type": "compact", "message": e.Message})
			}

		case agent.RetryEvent:
			if isJSON {
				emitJSON(map[string]any{"type": "retry", "reason": e.Reason})
			}
		}
	}

	return nil
}

func emitJSON(v any) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}

func loadPrintInstructions(wd string) string {
	paths := []string{
		filepath.Join(wd, ".swiftx", "instructions.md"),
		filepath.Join(wd, "SWIFTX.md"),
	}
	var parts []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			parts = append(parts, string(data))
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildPrintSkillSection(catalog *skills.Catalog) string {
	if catalog == nil {
		return ""
	}
	metas := catalog.List()
	if len(metas) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Available Skills\n\n")
	for _, meta := range metas {
		desc := meta.Description
		if len(desc) > 200 {
			desc = desc[:200] + "…"
		}
		fmt.Fprintf(&sb, "- /%s: %s\n", meta.Name, desc)
	}
	return sb.String()
}
