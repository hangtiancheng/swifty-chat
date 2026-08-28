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

package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/commands"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/compact"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/file_history"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/history"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/hooks"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/mcp"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory/consolidation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory/extractor"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/plan_file"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/prompt"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/sandbox"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/skills"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/subagent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/teams"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/todo"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/worktree"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/width"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type appState int

const (
	stateProviderSelect appState = iota
	stateChat
	stateResume
)

type chatMessage struct {
	role          string
	content       string
	toolGroup     []toolBlockInfo
	subAgentBlock *subAgentBlock
	expanded      bool
}

type subAgentBlock struct {
	desc      string
	agentType string
	toolUses  []toolBlockInfo
	done      bool
	toolCount int
	totalTime float64
}

type toolBlockInfo struct {
	toolName  string
	args      map[string]any
	output    string
	isError   bool
	elapsed   float64
	collapsed bool
	loading   bool
}

type agentEventMsg struct {
	event agent.AgentEvent
	ch    <-chan agent.AgentEvent
}
type agentDoneMsg struct {
	ch <-chan agent.AgentEvent
}
type agentErrMsg struct{ err error }
type agentReadyMsg struct {
	ch <-chan agent.AgentEvent
}
type mailboxPollMsg struct{}
type mcpReadyMsg struct{ result mcp.ConnectResult }
type compactDoneMsg struct {
	message string
	err     error
}

// forkSkillDoneMsg is dispatched when a fork-mode Skill's sub-agent has
// reached LoopComplete (or failed). Update injects result as a single
// assistant chatMessage into the main conversation log so the user sees
// the sub-agent's final answer without it polluting the parent agent's
// context window.
type forkSkillDoneMsg struct {
	name   string
	result string
	err    error
}

type Model struct {
	providers        []config.ProviderConfig
	selectedProvider *config.ProviderConfig
	client           llm.Client
	registry         *tools.Registry
	ag               *agent.Agent

	state     appState
	streaming bool

	textarea textarea.Model
	viewport viewport.Model
	width    int
	height   int
	ready    bool

	providerCursor int

	conversation *conversation.Manager

	chatMessages []chatMessage
	toolBlocks   []toolBlockInfo
	streamBuf    string
	agentCh      <-chan agent.AgentEvent
	cancelStream context.CancelFunc

	totalInput  int
	totalOutput int

	permDialog   bool
	permToolName string
	permDesc     string
	permRespCh   chan<- agent.PermissionResponse
	permCursor   int

	cmdRegistry   *commands.Registry
	slashMenuOpen bool
	slashMatches  []*commands.Command
	slashCursor   int

	userScrolled  bool
	committedUpTo int
	bannerPrinted bool

	atMenuOpen bool
	atMatches  []string
	atCursor   int
	atPrefix   string

	spinner       spinner.Model
	thinking      bool
	thinkingStart time.Time
	thinkingDone  float64
	thinkingVerb  string

	instructionsContent string
	memoryContent       string

	mcpConfigs        []config.MCPServerConfig
	mcpMgr            *mcp.Manager
	mcpConnecting     bool
	mcpInstructions   string
	mcpInstructionsOK bool
	mcpServerInfo     string
	hookConfigs       []hooks.Hook

	historyEntries []string
	historyIndex   int
	historyDraft   string

	sessionID    string
	fileHistory  *file_history.History
	defaultTools tools.DefaultTools
	prePlanMode  permissions.PermissionMode

	planApprovalDialog bool
	planApprovalCursor int
	planApprovalInput  string

	rewindDialog       bool
	rewindSnapshots    []file_history.Snapshot
	rewindCursor       int
	rewindPhase        int // 0=select checkpoint, 1=select restore option
	rewindOptionCursor int

	askUserCh          chan tools.AskUserRequest
	subAgentProgressCh chan subagent.SubAgentProgress
	activeSubAgent     *subAgentBlock
	askUserDialog      bool
	askUserQuestions   []tools.Question
	askUserCursors     []int
	askUserSelected    []map[int]bool
	askUserOther       []string
	askUserQIdx        int
	askUserRespCh      chan tools.QuestionResponse
	askUserAnswered    map[int]string
	askUserOnSubmit    bool
	askUserSubmitIdx   int
	skillCatalog       *skills.Catalog
	// announcedSkills tracks skill names already communicated to the model.
	// The first system-reminder carries the full catalog; afterwards only new
	// additions are appended to avoid redundant context usage.
	announcedSkills    map[string]bool
	taskMgr            *subagent.TaskManager
	todoList           *todo.TaskList
	memoryMgr          *memory.Manager
	memoryExtractor    *extractor.Extractor
	memoryConsolidator *consolidation.Consolidator
	teamMgr            *teams.TeamManager

	sandboxDialog         bool                 // Whether the sandbox mode selection dialog is open
	sandboxCursor         int                  // Currently selected sandbox mode index
	sandboxCfg            config.SandboxConfig // Sandbox settings from the config file
	EnableCoordinatorMode bool                 // Coordinator mode configuration toggle
	ForkDisabled          bool                 // When fork is disabled, omitting subagent_type falls back to a generic agent

	resumeSessions  []session.SessionInfo
	resumeFiltered  []session.SessionInfo
	resumeCursor    int
	resumeSearch    string
	resumeScrollTop int

	hasExitedPlanMode bool   // Records whether Plan Mode was exited during this session, used to inject a reminder on re-entry
	PermissionMode    string // Initial permission mode from the config file; invalid or empty values fall back to default
}

// asPermissionMode resolves the configured permission mode, falling back
// to ModeDefault when the value is empty or not a recognized mode.
func (m *Model) asPermissionMode() permissions.PermissionMode {
	switch mode := permissions.PermissionMode(m.PermissionMode); mode {
	case permissions.ModeDefault, permissions.ModeAcceptEdits, permissions.ModePlan, permissions.ModeBypass:
		return mode
	default:
		return permissions.ModeDefault
	}
}

func New(providers []config.ProviderConfig, mcpConfigs []config.MCPServerConfig, hookConfigs []hooks.Hook, sandboxCfg ...config.SandboxConfig) Model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.SetHeight(1)

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    80 * time.Millisecond,
	}
	sp.Style = lipgloss.NewStyle().Foreground(brandPurple)

	askCh := make(chan tools.AskUserRequest, 1)
	subProgressCh := make(chan subagent.SubAgentProgress, 32)
	dt := tools.CreateDefaultTools()
	reg := dt.Registry
	reg.Register(&tools.AskUserQuestionTool{RequestCh: askCh})

	var sCfg config.SandboxConfig
	if len(sandboxCfg) > 0 {
		sCfg = sandboxCfg[0]
	}

	m := Model{
		providers:          providers,
		mcpConfigs:         mcpConfigs,
		hookConfigs:        hookConfigs,
		sandboxCfg:         sCfg,
		state:              stateProviderSelect,
		textarea:           ta,
		conversation:       conversation.NewManager(),
		registry:           reg,
		defaultTools:       dt,
		cmdRegistry:        commands.CreateDefaultRegistry(),
		spinner:            sp,
		askUserCh:          askCh,
		subAgentProgressCh: subProgressCh,
	}

	if len(providers) == 1 {
		m.state = stateChat
	}

	return m
}

type initSingleProviderMsg struct{}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink}
	if len(m.providers) == 1 {
		cmds = append(cmds, func() tea.Msg { return initSingleProviderMsg{} })
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - 4)

		statusHeight := 1
		sepHeight := 2 // top + bottom separators around input
		inputHeight := m.textarea.Height() + 1
		vpHeight := max(msg.Height-statusHeight-sepHeight-inputHeight-1, 1)

		if !m.ready {
			m.viewport = viewport.New(msg.Width, vpHeight)
			m.viewport.MouseWheelEnabled = false
			m.ready = true
			m.bannerPrinted = true
			m.updateViewport()
			return m, tea.Println(m.renderBanner() + "\n")
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpHeight
		}
		m.updateViewport()
		return m, nil

	case subAgentProgressMsg:
		p := msg.progress
		if p.Done {
			if m.activeSubAgent != nil {
				m.activeSubAgent.done = true
				m.activeSubAgent.toolCount = p.ToolCount
				m.activeSubAgent.totalTime = p.TotalTime
			}
		} else {
			if m.activeSubAgent == nil || m.activeSubAgent.done {
				m.activeSubAgent = &subAgentBlock{
					desc:      p.AgentDesc,
					agentType: p.AgentType,
				}
			}
			m.activeSubAgent.toolUses = append(m.activeSubAgent.toolUses, toolBlockInfo{
				toolName: p.ToolName,
				args:     p.ToolArgs,
				elapsed:  p.Elapsed,
				isError:  p.IsError,
			})
		}
		m.updateViewport()
		return m, m.listenForSubAgentProgress()

	case askUserMsg:
		m.askUserDialog = true
		m.askUserQuestions = msg.req.Questions
		m.askUserRespCh = msg.req.ResponseCh
		m.askUserQIdx = 0
		m.askUserCursors = make([]int, len(msg.req.Questions))
		m.askUserSelected = make([]map[int]bool, len(msg.req.Questions))
		m.askUserOther = make([]string, len(msg.req.Questions))
		m.askUserAnswered = make(map[int]string)
		m.askUserOnSubmit = false
		m.askUserSubmitIdx = 0
		for i := range msg.req.Questions {
			m.askUserSelected[i] = make(map[int]bool)
		}
		m.updateViewport()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.streaming && m.cancelStream != nil {
				m.cancelStream()
				m.savePartialResponse()
				m.finishStreaming()
				return m, nil
			}
			if m.cancelStream != nil {
				m.cancelStream()
			}
			if m.mcpMgr != nil {
				m.mcpMgr.Shutdown()
			}
			if m.memoryExtractor != nil {
				_ = m.memoryExtractor.Drain(5000)
			}
			return m, tea.Quit
		}

		if m.permDialog {
			return m.handlePermDialog(msg)
		}

		switch m.state {
		case stateProviderSelect:
			return m.handleProviderSelect(msg)
		case stateChat:
			return m.handleChat(msg)
		case stateResume:
			return m.handleResumeKeys(msg)
		}

	case initSingleProviderMsg:
		p := &m.providers[0]
		m.selectedProvider = p
		wd, _ := os.Getwd()
		systemPrompt := m.loadSkillsAndBuildPrompt(wd)
		client, err := llm.NewClient(p, systemPrompt)
		if err != nil {
			m.chatMessages = append(m.chatMessages, chatMessage{role: "error", content: err.Error()})
			m.updateViewport()
			return m, nil
		}
		m.client = client
		m.sessionID = session.NewID()
		m.fileHistory = file_history.New(wd, m.sessionID)
		m.defaultTools.EditFile.FileHistory = m.fileHistory
		m.defaultTools.WriteFile.FileHistory = m.fileHistory
		m.registerAgentTools(client, p, p.Protocol, wd)
		// Best-effort: pull the model's context window from the provider once
		// (Anthropic only) and cache it on p before GetContextWindow reads it.
		// Silently degrades to the mapping table / default on any failure.
		llm.ResolveContextWindow(context.Background(), p)
		ag := agent.New(client, m.registry, p.Protocol)
		ag.ContextWindow = p.GetContextWindow()
		ag.MaxOutputTokens = p.GetMaxOutputTokens()
		ag.Instructions = m.instructionsContent
		ag.MemoryContent = m.memoryContent
		// The first system-reminder carries the full skill catalog; afterwards
		// SkillDeltaFn appends only newly added skills
		ag.SkillSection = m.skillDelta()
		ag.SkillDeltaFn = m.skillDelta
		ag.FileHistory = m.fileHistory
		ag.SetSessionID(m.sessionID)
		sandboxAllow := []string{memory.GetAutoMemPath(wd)}
		if userMem := memory.GetUserAutoMemPath(); userMem != "" {
			sandboxAllow = append(sandboxAllow, userMem)
		}
		pathSandbox := permissions.NewPathSandbox(wd, sandboxAllow...)
		ag.Checker = permissions.NewChecker(
			pathSandbox,
			permissions.NewRuleEngine(wd),
			m.asPermissionMode(),
		)
		// Initialize the OS-level sandbox based on the config file
		if m.sandboxCfg.Enabled {
			sb := sandbox.New()
			if bashTool, ok := m.registry.Get("Bash").(*tools.BashTool); ok && sb != nil {
				bashTool.Sandbox = sb
				bashTool.SandboxConfig = sandbox.Config{
					AllowWrite:     pathSandbox.GetAllowedRoots(),
					DenyWrite:      pathSandbox.GetDenyWrite(),
					NetworkEnabled: m.sandboxCfg.NetworkEnabled,
				}
			}
			if m.sandboxCfg.AutoAllow {
				ag.Checker.SandboxEnabled = true
			}
		}
		ag.NotificationFn = m.drainTaskNotifications
		ag.ToolNameFilter = teams.CoordinatorToolFilter(m.EnableCoordinatorMode)
		ag.CoordinatorActiveFn = teams.CoordinatorActiveFn(m.EnableCoordinatorMode)
		if len(m.hookConfigs) > 0 {
			eng := hooks.NewEngine()
			eng.LoadHooks(m.hookConfigs)
			eng.AgentRunner = newAgentHookRunner(client)
			ag.Hooks = eng
		}
		m.ag = ag
		if at, ok := m.registry.Get("Agent").(*subagent.AgentTool); ok {
			at.ParentChecker = ag.Checker
		}
		m.wireSkillsToAgent()
		m.memoryExtractor = m.installMemoryExtractor(ag, wd, p.Protocol)
		m.historyEntries = history.Load(wd)
		m.textarea.Focus()
		m.updateViewport()
		return m, m.initMCPServersCmd()

	case forkSkillDoneMsg:
		var commit string
		if msg.err != nil {
			line := fmt.Sprintf("Skill %s (fork) failed: %v", msg.name, msg.err)
			m.chatMessages = append(m.chatMessages, chatMessage{role: "error", content: line})
			commit = errorStyle.Render("✗ " + line)
		} else {
			result := strings.TrimSpace(msg.result)
			if result == "" {
				result = fmt.Sprintf("(Skill %s returned no output)", msg.name)
			}
			m.chatMessages = append(m.chatMessages, chatMessage{role: "assistant", content: result})
			commit = m.renderMessagesRange(len(m.chatMessages)-1, len(m.chatMessages))
		}
		m.committedUpTo = len(m.chatMessages)
		m.updateViewport()
		if commit != "" {
			return m, tea.Println(commit)
		}
		return m, nil

	case compactDoneMsg:
		switch {
		case msg.err != nil:
			m.chatMessages = append(m.chatMessages, chatMessage{role: "error", content: "Compact failed: " + msg.err.Error()})
		case msg.message == "":
			m.chatMessages = append(m.chatMessages, chatMessage{role: "system", content: "Compact: no changes."})
		default:
			m.chatMessages = append(m.chatMessages, chatMessage{role: "system", content: "Compact: " + msg.message})
		}
		m.updateViewport()
		return m, nil

	case mcpReadyMsg:
		m.mcpConnecting = false
		m.mcpMgr = msg.result.Mgr
		for _, t := range msg.result.Tools {
			m.registry.Register(t)
		}
		var mcpPrintLines []string
		for _, errMsg := range msg.result.Errors {
			m.chatMessages = append(m.chatMessages, chatMessage{
				role:    "error",
				content: errMsg,
			})
			mcpPrintLines = append(mcpPrintLines, errorStyle.Render("✗ "+errMsg))
		}
		// All tools must be in place before measuring the schema-to-context ratio
		if p := m.selectedProvider; p != nil {
			mcp.DecideAndApply(m.registry, p.BaseURL, p.GetContextWindow())
		}
		registered := len(msg.result.Tools)
		if registered > 0 {
			m.mcpServerInfo = fmt.Sprintf("Connected to %d MCP server(s), %d tools registered", len(m.mcpConfigs)-len(msg.result.Errors), registered)
		}
		m.committedUpTo = len(m.chatMessages)
		// Build MCP instructions for system prompt injection
		if len(msg.result.Servers) > 0 {
			// Group registered tool names by server
			toolsByServer := make(map[string][]string)
			for _, t := range msg.result.Tools {
				toolName := t.Name()
				for _, srv := range msg.result.Servers {
					if strings.HasPrefix(toolName, mcp.MCPToolNamePrefix(srv.Name)) {
						toolsByServer[srv.Name] = append(toolsByServer[srv.Name], toolName)
						break
					}
				}
			}

			var mcpParts []string
			for _, srv := range msg.result.Servers {
				var sb strings.Builder
				fmt.Fprintf(&sb, "## %s\n", srv.Name)
				if srv.Instructions != "" {
					sb.WriteString(srv.Instructions)
					sb.WriteString("\n")
				}
				if toolNames, ok := toolsByServer[srv.Name]; ok && len(toolNames) > 0 {
					sb.WriteString("\nAvailable tools: ")
					sb.WriteString(strings.Join(toolNames, ", "))
				}
				mcpParts = append(mcpParts, sb.String())
			}
			m.mcpInstructions = "# MCP Server Instructions\n\nThe following MCP servers are connected. Use their tools when the user asks.\n\n" + strings.Join(mcpParts, "\n\n")
		}
		m.updateViewport()
		if len(mcpPrintLines) > 0 {
			return m, tea.Println(strings.Join(mcpPrintLines, "\n"))
		}
		return m, nil

	case spinner.TickMsg:
		if m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.updateViewport()
			return m, cmd
		}

	case agentReadyMsg:
		m.agentCh = msg.ch
		return m, m.listenForAgentEvents()

	case agentEventMsg:
		if msg.ch != m.agentCh {
			return m, nil
		}
		return m.handleAgentEvent(msg.event)

	case agentDoneMsg:
		if msg.ch != m.agentCh {
			return m, nil
		}
		m.finishStreaming()
		return m, nil

	case agentErrMsg:
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "error",
			content: msg.err.Error(),
		})
		m.finishStreaming()
		return m, nil

	case mailboxPollMsg:
		if m.streaming {
			return m, nil
		}
		notifications := teams.DrainLeadMailbox(m.teamMgr)
		if len(notifications) == 0 {
			return m, m.pollMailbox()
		}
		for _, note := range notifications {
			m.conversation.AddSystemReminder(note)
		}
		m.streaming = true
		m.thinking = true
		m.thinkingStart = time.Now()
		m.thinkingDone = 0
		m.thinkingVerb = randomVerb()
		m.streamBuf = ""
		m.toolBlocks = nil
		m.userScrolled = false
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelStream = cancel
		m.agentCh = m.ag.Run(ctx, m.conversation)
		m.updateViewport()
		return m, tea.Batch(
			m.listenForAgentEvents(),
			m.listenForAskUser(),
			m.listenForSubAgentProgress(),
			m.spinner.Tick,
		)
	}

	return m, nil
}

func (m *Model) drainTaskNotifications() []string {
	var messages []string
	if m.taskMgr != nil {
		for _, n := range m.taskMgr.DrainNotifications() {
			msg := fmt.Sprintf("<task-notification>\n<task_id>%s</task_id>\n<status>%s</status>\n<summary>Agent \"%s\" %s</summary>\n<result>%s</result>\n</task-notification>",
				n.TaskID, n.Status, n.Name, n.Status, n.Output)
			messages = append(messages, msg)
		}
	}
	// Teammate idle notifications land in the lead's inbox; surface
	// them as system reminders so the Lead model sees them at the top
	// of the next turn and can dispatch follow-up work.
	messages = append(messages, teams.DrainLeadMailbox(m.teamMgr)...)
	// Hook notifications (post_tool_use output, async hook results, etc.)
	// drain into system reminders so the model sees side-effects.
	if m.ag != nil && m.ag.Hooks != nil {
		for _, r := range m.ag.Hooks.DrainNotifications() {
			if r.Output == "" || r.Output == "(async)" {
				continue
			}
			messages = append(messages, fmt.Sprintf("<hook-notification id=%q>\n%s\n</hook-notification>", r.HookID, r.Output))
		}
	}
	return messages
}

// newAgentHookRunner builds the AgentRunner closure used by `type: agent`
// hooks. The hook prompt is sent as a single user message to the same LLM
// the main agent uses, with no tool registry — output is the raw assistant
// text, which lands back in the notification queue and drains into the
// next turn's system reminders.
func newAgentHookRunner(client llm.Client) func(prompt string, ctx hooks.HookContext) (string, error) {
	return func(prompt string, _ hooks.HookContext) (string, error) {
		c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		conv := conversation.NewManager()
		conv.AddUserMessage(prompt)
		events, errs := client.Stream(c, conv, nil)
		var text strings.Builder
		for ev := range events {
			if td, ok := ev.(llm.TextDelta); ok {
				text.WriteString(td.Text)
			}
		}
		select {
		case err := <-errs:
			if err != nil {
				return "", err
			}
		default:
		}
		return text.String(), nil
	}
}

func (m *Model) registerAgentTools(client llm.Client, providerCfg *config.ProviderConfig, protocol, wd string) {
	m.taskMgr = subagent.NewTaskManager()

	store := todo.NewStore(wd, m.sessionID)
	m.todoList = todo.NewTaskList(store)

	m.memoryMgr = memory.NewManager(wd)

	loader := subagent.NewAgentLoader(wd)
	loader.LoadAll()

	teamMgr := teams.NewTeamManager()
	m.teamMgr = teamMgr

	// Wire worktree tools (T9: session restore, T13-T15: LLM tools, T17: cleanup)
	gitRoot := worktree.FindCanonicalGitRoot(wd)
	m.registry.Register(&tools.EnterWorktreeTool{
		SessionID: m.sessionID,
		RepoRoot:  gitRoot,
	})
	m.registry.Register(&tools.ExitWorktreeTool{
		RepoRoot: gitRoot,
	})

	// Restore worktree session from previous crash (T9)
	if gitRoot != "" {
		if savedSession, err := worktree.LoadWorktreeSession(gitRoot); err == nil && savedSession != nil {
			if info, err := os.Stat(savedSession.WorktreePath); err == nil && info.IsDir() {
				worktree.RestoreWorktreeSession(savedSession)
			}
		}
	}

	// Start background stale worktree cleanup (T17)
	worktree.StartCleanupLoop(context.Background())

	m.registry.Register(&tools.ExitPlanModeTool{
		IsPlanMode: func() bool {
			return m.ag != nil && m.ag.Checker != nil && m.ag.Checker.Mode == permissions.ModePlan
		},
		PlanExists: func() bool {
			wd, _ := os.Getwd()
			return plan_file.PlanExists(wd)
		},
	})
	m.registry.Register(&todo.TaskCreateTool{List: m.todoList})
	m.registry.Register(&todo.TaskGetTool{List: m.todoList})
	m.registry.Register(&todo.TaskListTool{List: m.todoList})
	m.registry.Register(&todo.TaskUpdateTool{List: m.todoList})
	m.registry.Register(&tools.ToolSearchTool{Registry: m.registry, Protocol: protocol})
	// McpCall must be registered before the MCP connection is established.
	// Waiting until after connection to decide whether to register it based
	// on loading mode would itself be a mid-session tools[] change, breaking
	// the cache prefix.
	m.registry.Register(&tools.McpCallTool{Registry: m.registry})
	m.registry.Register(&teams.TeamCreateTool{TeamMgr: teamMgr})
	m.registry.Register(&teams.TeamDeleteTool{TeamMgr: teamMgr})
	m.registry.Register(&teams.SendMessageTool{TeamMgr: teamMgr, SenderName: "lead"})
	m.registry.Register(&teams.TaskStopTool{TeamMgr: teamMgr})
	m.registry.Register(&tools.SyntheticOutputTool{})
	m.registry.Register(&subagent.AgentTool{
		Client:        client,
		ModelResolver: llm.NewModelResolver(*providerCfg),
		Registry:      m.registry,
		Protocol:      protocol,
		TaskMgr:       m.taskMgr,
		ProgressCh:    m.subAgentProgressCh,
		Loader:        loader,
		Conversation:  m.conversation,
		TeamMgr:       teamMgr,
		ForkDisabled:  m.ForkDisabled,
		// ParentChecker is wired below once m.ag.Checker is constructed —
		// registerAgentTools runs before the main agent's Checker is set.
	})

}

func (m *Model) initMCPServersCmd() tea.Cmd {
	if len(m.mcpConfigs) == 0 {
		return nil
	}

	m.mcpConnecting = true
	configs := m.mcpConfigs

	return func() tea.Msg {
		mgr := mcp.NewManager()
		var serverConfigs []mcp.ServerConfig
		for _, c := range configs {
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
		return mcpReadyMsg{result: mgr.ConnectAll(context.Background())}
	}
}

func (m *Model) loadSkillsAndBuildPrompt(wd string) string {
	m.skillCatalog = skills.LoadCatalog(wd)

	for _, cmd := range commands.LoadUserCommands(wd) {
		if m.cmdRegistry.HasConflict(cmd) {
			continue
		}
		m.cmdRegistry.Register(cmd)
	}

	return m.rebuildSystemPrompt(wd)
}

// wireSkillsToAgent finishes the Skill bring-up that loadSkillsAndBuildPrompt
// can't do because the Agent isn't constructed yet: registers per-Skill
// slash commands (inline vs fork-mode dispatch differs) and installs
// LoadSkillTool. Must be called immediately after m.ag is assigned and
// before the first user input is processed.
//
// Idempotent: silently skips a slash-command name that's already taken,
// matching the LoadUserCommands precedence in loadSkillsAndBuildPrompt and
// honoring the project rule "don't change the existing command set".
func (m *Model) wireSkillsToAgent() {
	if m.skillCatalog == nil || m.ag == nil {
		return
	}
	for _, meta := range m.skillCatalog.List() {
		m.registerSkillCommand(meta.Name)
	}
	m.registry.Register(&skills.LoadSkillTool{
		Catalog:  m.skillCatalog,
		Host:     m,
		ForkHost: m,
	})
	m.registry.Register(&skills.InstallSkillTool{
		Catalog: m.skillCatalog,
		OnInstalled: func(name string) {
			// Only register the slash command; leave the system prompt untouched.
			// The newly installed skill will be appended by skillDelta in the next
			// turn via a system-reminder.
			m.registerSkillCommand(name)
		},
	})
}

// registerSkillCommand wires a single skill's slash command. Inline skills
// route through TypePrompt + skills.RunInline so the SOP gets pinned and
// allowed_tools filtering kicks in. Fork skills route through TypeSkillFork
// so the dispatcher can offload to a goroutine + sub-agent.
//
// Idempotent: returns silently if the command name is already taken.
// Extracted from wireSkillsToAgent so InstallSkillTool's OnInstalled hook
// can re-register a single newly-fetched skill without re-running the full
// startup loop.
func (m *Model) registerSkillCommand(name string) {
	if m.skillCatalog == nil || m.cmdRegistry == nil {
		return
	}
	if m.cmdRegistry.Find(name) != nil {
		return
	}
	meta := m.skillCatalog.Get(name)
	if meta == nil {
		return
	}
	cmd := &commands.Command{
		Name:        name,
		Description: meta.Meta.Description + " [skill]",
	}
	if meta.Meta.IsFork() {
		cmd.Type = commands.TypeSkillFork
		cmd.Handler = func(ctx *commands.Context) string {
			// Handler is unused for fork dispatch — executeCommand
			// branches on TypeSkillFork before calling Handler. Keep
			// it non-nil so legacy code paths that gate on Handler
			// presence still work.
			return ""
		}
	} else {
		cmd.Type = commands.TypePrompt
		captured := name
		cmd.Handler = func(ctx *commands.Context) string {
			skill, err := m.skillCatalog.GetFull(captured)
			if err != nil && skill == nil {
				return fmt.Sprintf("[skill error] %v", err)
			}
			body, runErr := skills.RunInline(context.Background(), skill, ctx.Args, m)
			if runErr != nil {
				return fmt.Sprintf("[skill error] %v", runErr)
			}
			if m.ag != nil {
				m.ag.RecoveryState.RecordSkillInvocation(skill.Meta.Name, body)
			}
			return body
		}
	}
	m.cmdRegistry.Register(cmd)
}

// refreshSkillsIfNeeded checks whether the skill directories have changed
// since the catalog was last loaded. If so, it reloads the catalog and
// registers any new slash commands. The system prompt is left untouched:
// newly added skills are appended by skillDelta in the next turn via a
// system-reminder — modifying the system prompt would invalidate the entire
// cache prefix.
func (m *Model) refreshSkillsIfNeeded() {
	if m.skillCatalog == nil || m.client == nil {
		return
	}
	if !m.skillCatalog.NeedsReload() {
		return
	}
	wd, _ := os.Getwd()
	m.skillCatalog.Reload(wd)
	for _, meta := range m.skillCatalog.List() {
		m.registerSkillCommand(meta.Name)
	}
}

// skillDelta returns the listing of skills not yet announced to the model and
// records them in announcedSkills. The first system-reminder at session start
// already carries the full catalog; this only appends skills added afterwards.
func (m *Model) skillDelta() string {
	if m.skillCatalog == nil {
		return ""
	}
	if m.announcedSkills == nil {
		m.announcedSkills = map[string]bool{}
	}
	var lines []string
	for _, meta := range m.skillCatalog.List() {
		if m.announcedSkills[meta.Name] {
			continue
		}
		m.announcedSkills[meta.Name] = true
		desc := meta.Description
		if len(desc) > 200 {
			desc = desc[:200] + "…"
		}
		lines = append(lines, fmt.Sprintf("- /%s: %s", meta.Name, desc))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// rebuildSystemPrompt builds the system prompt. It is called once at session
// start and never rebuilt afterwards.
//
// The system prompt contains only project-agnostic product definitions (role,
// tool usage, behavioral guidelines) so it remains a single global copy with
// stable cache hits across projects. The three project-scoped items are not here:
// instructions, auto-memory, and the skill catalog are injected into the first
// message via conversation.InjectLongTermMemory as a system-reminder, and skills
// added mid-session are appended by skillDelta.
func (m *Model) rebuildSystemPrompt(wd string) string {
	m.instructionsContent = m.loadCustomInstructions(wd)
	m.memoryContent = memory.LoadAutoMemoryPrompt(wd)
	env := prompt.DetectEnvironment(wd)
	if m.selectedProvider != nil {
		env.Model = m.selectedProvider.Model
	}
	return prompt.BuildSystemPrompt(env)
}

// buildSkillSection generates the "## Available Skills" prompt section from
// the current catalog. Extracted from loadSkillsAndBuildPrompt so it can be
// reused by rebuildSystemPrompt.
func (m *Model) buildSkillSection(wd string) string {
	if m.skillCatalog == nil {
		return ""
	}
	metas := m.skillCatalog.List()
	if len(metas) == 0 {
		return ""
	}
	skillsDir := filepath.Join(wd, ".swiftx", "skills")
	var sb strings.Builder
	sb.WriteString("## Available Skills\n\n")
	fmt.Fprintf(&sb, "Skills are installed at: %s\n", skillsDir)
	sb.WriteString("When creating new skills, always place them under this directory as <skill-name>/SKILL.md.\n\n")
	sb.WriteString("Only Skill names and one-line descriptions are listed below. To activate a Skill on demand call the LoadSkill tool with {name: \"<skill-name>\"}. After activation the Skill's full SOP gets pinned to the environment context, and any tools the Skill declares get registered. Users can also invoke a Skill directly with /<name>.\n\n")
	sb.WriteString("If the user pastes a Skill URL (skills.sh, github.com tree URL, or raw SKILL.md URL) and asks to install / add / get it, call the InstallSkill tool with {url: \"<url>\"} — the new Skill becomes available immediately afterwards.\n\n")
	for _, meta := range metas {
		desc := meta.Description
		if len(desc) > 200 {
			desc = desc[:200] + "…"
		}
		fmt.Fprintf(&sb, "- /%s: %s\n", meta.Name, desc)
	}
	return sb.String()
}

// ----- SkillForkHost implementation on *Model -----

// ActivateSkill delegates to the underlying Agent so RunInline can pin the
// SOP to env context. Safe to call before m.ag exists (no-op).
func (m Model) ActivateSkill(name, body string) {
	if m.ag != nil {
		m.ag.ActivateSkill(name, body)
	}
}

// SetToolFilter installs a tool visibility filter on the Agent (used by
// Teams coordinator mode). Passing nil clears the filter.
func (m Model) SetToolFilter(allow func(name string) bool) {
	if m.ag == nil {
		return
	}
	m.ag.SetToolFilter(allow)
}

// ToolRegistry exposes the live registry for fail-fast checks and
// directory-type tool registration.
func (m Model) ToolRegistry() *tools.Registry {
	return m.registry
}

// SnapshotParentMessages copies the current main-conversation message log so
// the fork executor can seed the sub-agent per fork_context. Returns a
// shallow copy; callers must not mutate the slice.
func (m Model) SnapshotParentMessages() []conversation.Message {
	if m.conversation == nil {
		return nil
	}
	src := m.conversation.GetMessages()
	out := make([]conversation.Message, len(src))
	copy(out, src)
	return out
}

// RunSubAgent runs `body` as the first user message in an isolated
// sub-agent. The sub-agent gets a filtered registry honoring allowedTools
// (system tools always pass) and the same LLM client / protocol as the
// main loop. Blocks until the sub-agent reaches LoopComplete or errors;
// returns the final assistant text.
//
// Caller is expected to dispatch this on a goroutine (via tea.Cmd) — the
// channel drain here is synchronous and will freeze the UI if invoked on
// the bubbletea Update path.
func (m Model) RunSubAgent(ctx context.Context, body string, seed []conversation.Message, _ string) (string, error) {
	if m.client == nil {
		return "", fmt.Errorf("RunSubAgent: no llm client (provider not selected)")
	}
	subConv := conversation.NewManager()
	for _, msg := range seed {
		switch msg.Role {
		case "user":
			subConv.AddUserMessage(msg.Content)
		case "assistant":
			subConv.AddAssistantMessage(msg.Content)
		}
	}
	subConv.AddUserMessage(body)

	subAgent := agent.New(m.client, m.registry, "")
	if m.selectedProvider != nil {
		subAgent.Protocol = m.selectedProvider.Protocol
		subAgent.ContextWindow = m.selectedProvider.GetContextWindow()
		subAgent.MaxOutputTokens = m.selectedProvider.GetMaxOutputTokens()
	}
	subAgent.MaxIterations = 50

	var output strings.Builder
	ch := subAgent.Run(ctx, subConv)
	for ev := range ch {
		switch e := ev.(type) {
		case agent.StreamText:
			output.WriteString(e.Text)
		case agent.ErrorEvent:
			return output.String(), fmt.Errorf("%s", e.Message)
		}
	}
	return output.String(), nil
}

func (m *Model) loadCustomInstructions(wd string) string {
	return memory.LoadInstructions(wd)
}

// installMemoryExtractor wires ch09 background memory extraction onto the
// given agent. Constructs an Extractor with the current TUI context and
// hooks it onto ag.OnLoopComplete. Returns the Extractor so the caller
// can store it on Model.memoryExtractor for later Drain.
func (m *Model) installMemoryExtractor(ag *agent.Agent, wd, protocol string) *extractor.Extractor {
	if m.client == nil || m.conversation == nil {
		return nil
	}
	conv := m.conversation
	extr := extractor.InitExtractMemories(extractor.Deps{
		MemoryDir:     memory.GetAutoMemPath(wd),
		UserMemoryDir: memory.GetUserAutoMemPath(),
		ProjectRoot:   wd,
		Client:        m.client,
		ToolRegistry:  m.registry,
		Protocol:      protocol,
		Conversation:  conv,
		AppendSystem:  func(s string) { conv.AddSystemReminder(s) },
	})

	// Memory consolidator: automatically merges duplicates, removes stale entries, and resolves contradictions in the background
	consolidator := consolidation.NewConsolidator(consolidation.Deps{
		MemoryDir:     memory.GetAutoMemPath(wd),
		UserMemoryDir: memory.GetUserAutoMemPath(),
		ProjectRoot:   wd,
		Client:        m.client,
		ToolRegistry:  m.registry,
		Protocol:      protocol,
		Conversation:  conv,
		AppendSystem:  func(s string) { conv.AddSystemReminder(s) },
	})
	m.memoryConsolidator = consolidator

	ag.OnLoopComplete = func(_ *conversation.Manager) {
		_ = extr.Execute(context.Background())
		consolidator.MaybeRun(context.Background())
	}
	return extr
}

// prefetchRelevantMemories runs the recall selector in a goroutine and
// returns a channel that will receive the rendered system-reminder plus
// the selected memory paths (empty if nothing was selected / selector
// timed out). The agent loop reads it at most once, without blocking.
//
// Fires a fresh side-query llm.Client per call so the selector's
// SYSTEM prompt is independent of the main conversation's system prompt.
func (m *Model) prefetchRelevantMemories(query string) <-chan agent.RecallResult {
	out := make(chan agent.RecallResult, 1)
	if m.memoryMgr == nil || m.selectedProvider == nil {
		out <- agent.RecallResult{}
		return out
	}
	provider := m.selectedProvider
	memDir := m.memoryMgr.Dir()
	userMemDir := m.memoryMgr.UserDir()
	// Capture the agent pointer before spawning the goroutine; inside the
	// goroutine only its concurrency-safe methods are accessed.
	ag := m.ag
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		selector := func(ctx context.Context, sys, user string) (string, error) {
			sideClient, err := llm.NewClient(provider, sys)
			if err != nil {
				return "", err
			}
			miniConv := conversation.NewManager()
			miniConv.AddUserMessage(user)
			events, errs := sideClient.Stream(ctx, miniConv, nil)
			var sb strings.Builder
			for ev := range events {
				if td, ok := ev.(llm.TextDelta); ok {
					sb.WriteString(td.Text)
				}
			}
			select {
			case err := <-errs:
				if err != nil {
					return "", err
				}
			default:
			}
			return sb.String(), nil
		}
		// Recently used tools let the selector skip their usage guides;
		// already-injected memories are excluded outright to prevent the same
		// entry from consuming a slot every turn.
		var recentTools []string
		var surfaced map[string]struct{}
		if ag != nil {
			recentTools, surfaced = ag.RecallHints()
		}
		results, _ := memory.FindRelevantMemories(ctx, query, userMemDir, memDir, recentTools, surfaced, selector)
		// Select and render only; selected paths travel with the result to the
		// agent loop and are marked as surfaced upon injection.
		paths := make([]string, 0, len(results))
		for _, r := range results {
			paths = append(paths, r.Path)
		}
		out <- agent.RecallResult{Reminder: renderRelevantMemoriesReminder(results), Paths: paths}
	}()
	return out
}

// renderRelevantMemoriesReminder formats up to 5 recalled memory files
// as a single system-reminder body. Each memory gets a freshness header
// (today / N days ago) and its file content inline. Files that fail to
// read are silently skipped.
func renderRelevantMemoriesReminder(memories []memory.RelevantMemory) string {
	if len(memories) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("The following relevant memories from prior conversations may help:\n\n")
	for _, mem := range memories {
		data, err := os.ReadFile(mem.Path)
		if err != nil {
			continue
		}
		fmt.Fprintf(&sb, "## Memory: %s (saved %s)\n\n", filepath.Base(mem.Path), memory.MemoryAge(mem.MtimeMs))
		if note := memory.MemoryFreshnessText(mem.MtimeMs); note != "" {
			sb.WriteString(note)
			sb.WriteString("\n\n")
		}
		sb.Write(data)
		sb.WriteString("\n\n---\n\n")
	}
	return sb.String()
}

func (m *Model) savePartialResponse() {
	if m.streamBuf != "" {
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "assistant",
			content: m.streamBuf,
		})
		m.conversation.AddAssistantMessage(m.streamBuf)
		wd, _ := os.Getwd()
		session.SaveMessage(wd, m.sessionID, session.Message{
			Role: "assistant", Content: m.streamBuf, Ts: time.Now().Unix(),
		})
		m.streamBuf = ""
	}
	m.toolBlocks = nil

	msgs := m.conversation.GetMessages()
	if len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		if last.Role == "assistant" && len(last.ToolUses) > 0 {
			var results []conversation.ToolResultBlock
			for _, tu := range last.ToolUses {
				results = append(results, conversation.ToolResultBlock{
					ToolUseID: tu.ToolUseID,
					Content:   "Tool execution was interrupted by user.",
					IsError:   true,
				})
			}
			m.conversation.AddToolResultsMessage(results)
		}
	}

	m.chatMessages = append(m.chatMessages, chatMessage{
		role:    "system",
		content: "(response interrupted)",
	})
	m.updateViewport()
}

func (m *Model) finishStreaming() {
	if m.thinking {
		m.thinkingDone = time.Since(m.thinkingStart).Seconds()
		m.thinking = false
	}
	m.streaming = false
	m.cancelStream = nil
	m.agentCh = nil
	m.updateViewport()
}

func (m Model) handleProviderSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.providerCursor > 0 {
			m.providerCursor--
		}
	case "down", "j":
		if m.providerCursor < len(m.providers)-1 {
			m.providerCursor++
		}
	case "enter":
		p := &m.providers[m.providerCursor]
		m.selectedProvider = p
		wd, _ := os.Getwd()
		systemPrompt := m.loadSkillsAndBuildPrompt(wd)
		client, err := llm.NewClient(p, systemPrompt)
		if err != nil {
			m.state = stateChat
			m.chatMessages = append(m.chatMessages, chatMessage{role: "error", content: err.Error()})
			return m, nil
		}
		m.client = client
		m.sessionID = session.NewID()
		m.fileHistory = file_history.New(wd, m.sessionID)
		m.defaultTools.EditFile.FileHistory = m.fileHistory
		m.defaultTools.WriteFile.FileHistory = m.fileHistory
		m.registerAgentTools(client, p, p.Protocol, wd)
		// Best-effort: pull the model's context window from the provider once
		// (Anthropic only) and cache it on p before GetContextWindow reads it.
		// Silently degrades to the mapping table / default on any failure.
		llm.ResolveContextWindow(context.Background(), p)
		ag := agent.New(client, m.registry, p.Protocol)
		ag.ContextWindow = p.GetContextWindow()
		ag.MaxOutputTokens = p.GetMaxOutputTokens()
		ag.Instructions = m.instructionsContent
		ag.MemoryContent = m.memoryContent
		ag.SkillSection = m.skillDelta()
		ag.SkillDeltaFn = m.skillDelta
		ag.FileHistory = m.fileHistory
		ag.SetSessionID(m.sessionID)
		sandboxAllow := []string{memory.GetAutoMemPath(wd)}
		if userMem := memory.GetUserAutoMemPath(); userMem != "" {
			sandboxAllow = append(sandboxAllow, userMem)
		}
		pathSandbox2 := permissions.NewPathSandbox(wd, sandboxAllow...)
		ag.Checker = permissions.NewChecker(
			pathSandbox2,
			permissions.NewRuleEngine(wd),
			m.asPermissionMode(),
		)
		// Initialize the OS-level sandbox based on the config file
		if m.sandboxCfg.Enabled {
			sb := sandbox.New()
			if bashTool, ok := m.registry.Get("Bash").(*tools.BashTool); ok && sb != nil {
				bashTool.Sandbox = sb
				bashTool.SandboxConfig = sandbox.Config{
					AllowWrite:     pathSandbox2.GetAllowedRoots(),
					DenyWrite:      pathSandbox2.GetDenyWrite(),
					NetworkEnabled: m.sandboxCfg.NetworkEnabled,
				}
			}
			if m.sandboxCfg.AutoAllow {
				ag.Checker.SandboxEnabled = true
			}
		}
		ag.NotificationFn = m.drainTaskNotifications
		ag.ToolNameFilter = teams.CoordinatorToolFilter(m.EnableCoordinatorMode)
		ag.CoordinatorActiveFn = teams.CoordinatorActiveFn(m.EnableCoordinatorMode)
		if len(m.hookConfigs) > 0 {
			eng := hooks.NewEngine()
			eng.LoadHooks(m.hookConfigs)
			eng.AgentRunner = newAgentHookRunner(client)
			ag.Hooks = eng
		}
		m.ag = ag
		if at, ok := m.registry.Get("Agent").(*subagent.AgentTool); ok {
			at.ParentChecker = ag.Checker
		}
		m.wireSkillsToAgent()
		m.memoryExtractor = m.installMemoryExtractor(ag, wd, p.Protocol)
		m.historyEntries = history.Load(wd)
		// NOTE: keep m.sessionID == the id wired into ag (SetSessionID) and into
		// m.fileHistory above; do NOT mint a fresh id here, or compact boundaries
		// would land in a different session file than the one the TUI appends to.
		m.state = stateChat
		m.textarea.Focus()
		m.updateViewport()
		return m, m.initMCPServersCmd()
	}
	return m, nil
}

func (m Model) handleChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.askUserDialog {
		return m.handleAskUserDialog(msg)
	}

	if m.planApprovalDialog {
		return m.handlePlanApproval(msg)
	}

	if m.rewindDialog {
		return m.handleRewindKeys(msg)
	}

	if m.sandboxDialog {
		return m.handleSandboxDialog(msg)
	}

	// ctrl+o: toggle expand/collapse on ALL collapsible blocks
	if msg.String() == "ctrl+o" {
		toggled := false
		for i := range m.chatMessages {
			r := m.chatMessages[i].role
			if r == "tool_group" || r == "tool_collapsed" || r == "sub_agent" {
				m.chatMessages[i].expanded = !m.chatMessages[i].expanded
				toggled = true
			}
		}
		if toggled {
			m.updateViewport()
		}
		return m, nil
	}

	// ESC during streaming: adopt running sub-agent to background
	if msg.String() == "escape" && m.streaming && m.agentCh != nil && m.cancelStream != nil {
		if m.taskMgr != nil {
			taskID := m.taskMgr.AdoptRunning("manual-background", m.agentCh, m.cancelStream)
			m.chatMessages = append(m.chatMessages, chatMessage{
				role:    "system",
				content: fmt.Sprintf("Agent moved to background (task %s). You will be notified when it completes.", taskID),
			})
			m.agentCh = nil
			m.cancelStream = nil
			m.finishStreaming()
			commitText := m.renderMessagesRange(m.committedUpTo, len(m.chatMessages))
			m.committedUpTo = len(m.chatMessages)
			if commitText != "" {
				return m, tea.Println(commitText)
			}
		}
		return m, nil
	}

	if m.atMenuOpen {
		switch msg.String() {
		case "up":
			if m.atCursor > 0 {
				m.atCursor--
			}
			return m, nil
		case "down":
			if m.atCursor < len(m.atMatches)-1 {
				m.atCursor++
			}
			return m, nil
		case "enter", "tab":
			if m.atCursor < len(m.atMatches) {
				selected := m.atMatches[m.atCursor]
				// Replace @prefix with @filepath
				text := m.textarea.Value()
				atIdx := strings.LastIndex(text, "@")
				if atIdx >= 0 {
					m.textarea.Reset()
					m.textarea.SetHeight(1)
					m.textarea.InsertString(text[:atIdx] + "@" + selected + " ")
				}
				m.atMenuOpen = false
				m.atMatches = nil
				m.atCursor = 0
			}
			return m, nil
		case "escape":
			m.atMenuOpen = false
			m.atMatches = nil
			m.atCursor = 0
			return m, nil
		}
	}

	if m.slashMenuOpen {
		switch msg.String() {
		case "up":
			if m.slashCursor > 0 {
				m.slashCursor--
			}
			return m, nil
		case "down":
			if m.slashCursor < len(m.slashMatches)-1 {
				m.slashCursor++
			}
			return m, nil
		case "enter":
			if m.slashCursor < len(m.slashMatches) {
				selected := m.slashMatches[m.slashCursor]
				m.slashMenuOpen = false
				m.slashMatches = nil
				m.slashCursor = 0
				m.textarea.Reset()
				m.textarea.SetHeight(1)
				return m.executeCommand(selected.Name, "")
			}
			return m, nil
		case "escape":
			m.slashMenuOpen = false
			m.slashMatches = nil
			m.slashCursor = 0
			return m, nil
		case "tab":
			if m.slashCursor < len(m.slashMatches) {
				selected := m.slashMatches[m.slashCursor]
				m.textarea.Reset()
				m.textarea.SetHeight(1)
				m.textarea.InsertString("/" + selected.Name + " ")
				m.slashMenuOpen = false
				m.slashMatches = nil
				m.slashCursor = 0
			}
			return m, nil
		}
	}

	if msg.String() == "shift+tab" {
		if m.ag != nil && m.ag.Checker != nil && !m.streaming {
			m.ag.Checker.Mode = nextPermissionMode(m.ag.Checker.Mode)
			m.updateViewport()
		}
		return m, nil
	}

	if msg.String() == "ctrl+j" {
		m.textarea.InsertString("\n")
		m.resizeTextarea()
		return m, nil
	}

	if msg.String() == "enter" {
		text := strings.TrimSpace(m.textarea.Value())
		if text == "" {
			return m, nil
		}
		if m.streaming {
			if m.cancelStream != nil {
				m.cancelStream()
			}
			m.savePartialResponse()
			m.finishStreaming()
		}
		if strings.HasPrefix(text, "/") {
			name, args := commands.Parse(text)
			m.textarea.Reset()
			m.textarea.SetHeight(1)
			m.slashMenuOpen = false
			m.slashMatches = nil
			m.slashCursor = 0
			return m.executeCommand(name, args)
		}
		m.slashMenuOpen = false
		return m.sendMessage(text)
	}

	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		m.userScrolled = !m.viewport.AtBottom()
		return m, vpCmd
	case "up":
		if !m.streaming && m.textarea.Line() == 0 {
			m.historyUp()
			return m, nil
		}
	case "down":
		if !m.streaming && m.textarea.Line() == m.textarea.LineCount()-1 {
			m.historyDown()
			return m, nil
		}
	}

	prevText := m.textarea.Value()
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	if m.textarea.Value() != prevText {
		m.historyIndex = 0
		m.resizeTextarea()
	}

	m.updateSlashMenu()
	m.updateAtMenu()

	return m, cmd
}

// stringWidth returns the approximate display width of s in terminal cells.
// Wide characters (CJK, fullwidth) count as 2, combining marks and control
// characters as 0, everything else as 1. This is used only for textarea height
// estimation, so minor inaccuracy on complex emoji sequences is acceptable.
func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case unicode.IsControl(r):
		case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r):
		case width.EastAsianWide == width.LookupRune(r).Kind(),
			width.EastAsianFullwidth == width.LookupRune(r).Kind():
			w += 2
		default:
			w += 1
		}
	}
	return w
}

func (m *Model) resizeTextarea() {
	content := m.textarea.Value()
	textWidth := max(m.textarea.Width(), 1)
	total := 0
	for line := range strings.SplitSeq(content, "\n") {
		w := stringWidth(line)
		if w <= textWidth {
			total++
		} else {
			total += (w + textWidth - 1) / textWidth
		}
	}
	maxH := max(m.height/2, 1)
	if total > maxH {
		total = maxH
	}
	if total < 1 {
		total = 1
	}
	m.textarea.SetHeight(total)
	m.updateViewport()
}

func (m *Model) updateSlashMenu() {
	text := m.textarea.Value()
	if !strings.HasPrefix(text, "/") || m.historyIndex > 0 {
		m.slashMenuOpen = false
		m.slashMatches = nil
		m.slashCursor = 0
		return
	}

	prefix := strings.TrimPrefix(text, "/")
	if strings.Contains(prefix, " ") {
		m.slashMenuOpen = false
		m.slashMatches = nil
		m.slashCursor = 0
		return
	}

	names := m.cmdRegistry.Complete(prefix)
	var matches []*commands.Command
	for _, name := range names {
		if cmd := m.cmdRegistry.Find(name); cmd != nil {
			seen := false
			for _, existing := range matches {
				if existing.Name == cmd.Name {
					seen = true
					break
				}
			}
			if !seen {
				matches = append(matches, cmd)
			}
		}
	}

	if len(matches) > 8 {
		matches = matches[:8]
	}
	m.slashMatches = matches
	m.slashMenuOpen = len(matches) > 0
	if m.slashCursor >= len(matches) {
		m.slashCursor = 0
	}
}

func (m *Model) updateAtMenu() {
	if m.slashMenuOpen {
		m.atMenuOpen = false
		return
	}

	text := m.textarea.Value()
	atIdx := strings.LastIndex(text, "@")
	if atIdx < 0 {
		m.atMenuOpen = false
		m.atMatches = nil
		m.atCursor = 0
		return
	}

	after := text[atIdx+1:]
	if strings.Contains(after, " ") {
		m.atMenuOpen = false
		m.atMatches = nil
		m.atCursor = 0
		return
	}

	m.atPrefix = after
	matches := scanFiles(after, 8)
	m.atMatches = matches
	m.atMenuOpen = len(matches) > 0
	if m.atCursor >= len(matches) {
		m.atCursor = 0
	}
}

func scanFiles(prefix string, limit int) []string {
	dir := "."
	searchPrefix := prefix

	if strings.Contains(prefix, "/") {
		lastSlash := strings.LastIndex(prefix, "/")
		dir = prefix[:lastSlash]
		if dir == "" {
			dir = "/"
		}
		searchPrefix = prefix[lastSlash+1:]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, ".venv": true,
		"__pycache__": true, ".swiftx": true, "vendor": true,
	}

	var matches []string
	for _, e := range entries {
		if skipDirs[e.Name()] {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") && searchPrefix == "" {
			continue
		}
		if searchPrefix != "" && !strings.HasPrefix(strings.ToLower(e.Name()), strings.ToLower(searchPrefix)) {
			continue
		}

		path := e.Name()
		if dir != "." {
			path = dir + "/" + e.Name()
		}
		if e.IsDir() {
			path += "/"
		}
		matches = append(matches, path)
		if len(matches) >= limit {
			break
		}
	}
	return matches
}

func (m Model) buildCommandContext(args string) *commands.Context {
	wd, _ := os.Getwd()
	modelName := ""
	if m.selectedProvider != nil {
		modelName = m.selectedProvider.Model
	}
	return &commands.Context{
		Args:    args,
		WorkDir: wd,
		Model:   modelName,
		MemoryList: func() []string {
			if m.memoryMgr == nil {
				return nil
			}
			return m.memoryMgr.GetMemories()
		},
		MemoryClear: func() {
			if m.memoryMgr != nil {
				m.memoryMgr.Clear()
			}
		},
		TokenCount: func() (int, int) {
			return m.totalInput, m.totalOutput
		},
		PermissionMode: func() string {
			if m.ag != nil && m.ag.Checker != nil {
				return string(m.ag.Checker.Mode)
			}
			return "default"
		},
		ToolCount: func() int {
			return len(m.registry.ListTools())
		},
		SessionInfo: func() string {
			return fmt.Sprintf("Current session: %d messages", len(m.chatMessages))
		},
		SkillList: func() []commands.SkillInfo {
			if m.skillCatalog == nil {
				return nil
			}
			var result []commands.SkillInfo
			for _, meta := range m.skillCatalog.List() {
				result = append(result, commands.SkillInfo{
					Name:        meta.Name,
					Description: meta.Description,
				})
			}
			return result
		},
		SkillReload: func() int {
			if m.skillCatalog == nil {
				return 0
			}
			wd, _ := os.Getwd()
			m.skillCatalog.Reload(wd)
			for _, meta := range m.skillCatalog.List() {
				m.registerSkillCommand(meta.Name)
			}
			// Do not rebuild the system prompt after reload; new skills are
			// appended by skillDelta in the next turn
			return len(m.skillCatalog.List())
		},
		MCPInfo: func() string {
			return m.mcpServerInfo
		},
	}
}

func (m Model) executeCommand(name, args string) (tea.Model, tea.Cmd) {
	cmd := m.cmdRegistry.Find(name)
	if cmd == nil {
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "error",
			content: fmt.Sprintf("Unknown command: /%s — type /help to see available commands", name),
		})
		m.updateViewport()
		return m, nil
	}

	if args == "" && cmd.ArgPrompt != "" {
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "system",
			content: cmd.ArgPrompt,
		})
		m.updateViewport()
		return m, nil
	}

	ctx := m.buildCommandContext(args)

	switch cmd.Type {
	case commands.TypeLocalUI:
		switch name {
		case "clear":
			m.chatMessages = nil
			m.committedUpTo = 0
			m.conversation = conversation.NewManager()
			if m.ag != nil {
				m.ag.ClearActiveSkills()
				// Skill tool narrowing is cleared along with the conversation, but the
				// coordinator constraint is not: it follows the Team — as long as the
				// Team is active, it should keep governing the Lead.
				m.ag.SetToolFilter(teams.CoordinatorToolFilter(m.EnableCoordinatorMode))
			}
			// Start a brand-new session: reset session ID and associated persistent storage
			wd, _ := os.Getwd()
			m.sessionID = session.NewID()
			m.fileHistory = file_history.New(wd, m.sessionID)
			m.defaultTools.EditFile.FileHistory = m.fileHistory
			m.defaultTools.WriteFile.FileHistory = m.fileHistory
			if m.ag != nil {
				m.ag.FileHistory = m.fileHistory
				m.ag.SetSessionID(m.sessionID)
			}
			store := todo.NewStore(wd, m.sessionID)
			m.todoList = todo.NewTaskList(store)
			// Reset token counters
			m.totalInput = 0
			m.totalOutput = 0
			m.updateViewport()
			return m, tea.Batch(
				func() tea.Msg { return tea.ClearScreen() },
				tea.Println(m.renderBanner()+"\n"),
			)
		case "plan":
			wd, _ := os.Getwd()
			if m.ag != nil && m.ag.Checker != nil {
				m.prePlanMode = m.ag.Checker.Mode
				m.ag.Checker.Mode = permissions.ModePlan
				planPath := plan_file.GetOrCreatePlanPath(wd)
				m.ag.Checker.PlanFilePath = planPath
				m.chatMessages = append(m.chatMessages, chatMessage{
					role:    "system",
					content: fmt.Sprintf("Entered Plan mode. Plan file: %s\nExplore the codebase and design your approach.", planPath),
				})

				// Re-entry detection: if Plan Mode was previously exited in this session
				// and the plan file already exists, inject a re-entry reminder
				if m.hasExitedPlanMode && plan_file.PlanExists(wd) {
					reentryMsg := prompt.BuildPlanModeReentryReminder(planPath, true)
					if reentryMsg != "" {
						m.chatMessages = append(m.chatMessages, chatMessage{
							role:    "system",
							content: reentryMsg,
						})
					}
					m.hasExitedPlanMode = false
				}
			}
			if args != "" {
				m.updateViewport()
				return m.sendMessage(args)
			}
			m.updateViewport()
			return m, nil
		case "compact":
			if m.client == nil || m.conversation == nil {
				m.chatMessages = append(m.chatMessages, chatMessage{
					role: "error", content: "Compact requires an active provider.",
				})
				m.updateViewport()
				return m, nil
			}
			m.chatMessages = append(m.chatMessages, chatMessage{
				role: "system", content: "Compacting conversation…",
			})
			m.updateViewport()
			client := m.client
			conv := m.conversation
			window := 200000
			if m.selectedProvider != nil {
				window = m.selectedProvider.GetContextWindow()
			}
			var recovery *compact.RecoveryState
			var schemas []map[string]any
			if m.ag != nil {
				recovery = m.ag.RecoveryState
				schemas = m.ag.Registry.GetAllSchemas(m.ag.Protocol)
			}
			compactWD, _ := os.Getwd()
			compactSessionID := m.sessionID
			return m, func() tea.Msg {
				msg, err := compact.ForceCompact(context.Background(), conv, client, compactWD, compactSessionID, window, recovery, schemas)
				return compactDoneMsg{message: msg, err: err}
			}
		case "resume":
			return m.handleResume(args)
		case "rewind":
			return m.handleRewind()
		case "sandbox":
			m.sandboxDialog = true
			m.sandboxCursor = 0
			m.updateViewport()
			return m, nil
		}

	case commands.TypePrompt:
		if cmd.Handler != nil {
			prompt := cmd.Handler(ctx)
			displayText := "/" + name
			if args != "" {
				displayText += " " + args
			}
			m.updateViewport()
			newModel, teaCmd := m.sendPromptCommand(displayText, prompt)
			if strings.HasSuffix(cmd.Description, "[skill]") {
				loadedLine := lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(
					fmt.Sprintf("skill(%s)\nSuccessfully loaded skill", name))
				return newModel, tea.Batch(tea.Println(loadedLine), teaCmd)
			}
			return newModel, teaCmd
		}

	case commands.TypeSkillFork:
		// Fork-mode skill: run the skill in an isolated sub-agent, show a
		// progress notice in the main chat, and inject the final assistant
		// text once the sub-agent reports back. Off-thread so the TUI
		// stays responsive while the sub-agent thinks.
		//
		// The two header lines (user echo + "Forking…" notice) get committed
		// to terminal scrollback via tea.Println the same way sendMessage
		// commits its userLine — without this the viewport keeps growing
		// during the sub-agent run and pushes earlier history above the fold.
		displayText := "/" + name
		if args != "" {
			displayText += " " + args
		}
		m.chatMessages = append(m.chatMessages, chatMessage{role: "user", content: displayText})
		userLine := promptStyle.Render("> ") + lipgloss.NewStyle().Foreground(brightText).Bold(true).Render(displayText)
		forkNotice := fmt.Sprintf("Forking skill %s in isolated sub-agent…", name)
		m.chatMessages = append(m.chatMessages, chatMessage{role: "system", content: forkNotice})
		m.committedUpTo = len(m.chatMessages)
		m.updateViewport()
		skillName := name
		skillArgs := args
		commitText := userLine + "\n" + lipgloss.NewStyle().Foreground(dimText).Render(forkNotice)
		return m, tea.Batch(
			tea.Println(commitText),
			func() tea.Msg {
				skill, err := m.skillCatalog.GetFull(skillName)
				if err != nil && skill == nil {
					return forkSkillDoneMsg{name: skillName, err: err}
				}
				if m.ag != nil {
					m.ag.RecoveryState.RecordSkillInvocation(skill.Meta.Name, skill.PromptBody)
				}
				result, runErr := skills.RunFork(context.Background(), skill, skillArgs, m)
				return forkSkillDoneMsg{name: skillName, result: result, err: runErr}
			},
		)

	case commands.TypeLocal:
		if cmd.Handler != nil {
			output := cmd.Handler(ctx)
			m.chatMessages = append(m.chatMessages, chatMessage{role: "system", content: output})
			m.updateViewport()
			return m, nil
		}
	}

	m.chatMessages = append(m.chatMessages, chatMessage{
		role:    "system",
		content: fmt.Sprintf("/%s — not yet implemented", name),
	})
	m.updateViewport()
	return m, nil
}

var permOptions = []struct {
	label    string
	response agent.PermissionResponse
}{
	{"Yes", agent.PermAllow},
	{"Yes, and don't ask again for this pattern", agent.PermAllowAlways},
	{"No", agent.PermDeny},
}

func (m Model) handlePermDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.permCursor > 0 {
			m.permCursor--
		}
		return m, nil
	case "down":
		if m.permCursor < len(permOptions)-1 {
			m.permCursor++
		}
		return m, nil
	case "enter":
		m.permRespCh <- permOptions[m.permCursor].response
		m.permDialog = false
		m.permCursor = 0
		m.updateViewport()
		return m, m.listenForAgentEvents()
	case "escape":
		m.permRespCh <- agent.PermDeny
		m.permDialog = false
		m.permCursor = 0
		m.updateViewport()
		return m, m.listenForAgentEvents()
	}
	return m, nil
}

func (m Model) handlePlanApproval(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.planApprovalCursor > 0 {
			m.planApprovalCursor--
		}
		m.updateViewport()
		return m, nil
	case "down", "j":
		if m.planApprovalCursor < 2 {
			m.planApprovalCursor++
		}
		m.updateViewport()
		return m, nil
	case "enter":
		if m.planApprovalCursor == 2 && m.planApprovalInput != "" {
			return m.sendPlanFeedback(m.planApprovalInput, false)
		}
		return m.executePlanApproval()
	case "shift+tab":
		if m.planApprovalCursor == 2 && m.planApprovalInput != "" {
			return m.sendPlanFeedback(m.planApprovalInput, true)
		}
		return m, nil
	case "escape":
		m.planApprovalDialog = false
		m.updateViewport()
		return m, nil
	case "backspace":
		if m.planApprovalCursor == 2 && len(m.planApprovalInput) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.planApprovalInput)
			m.planApprovalInput = m.planApprovalInput[:len(m.planApprovalInput)-size]
			m.updateViewport()
		}
		return m, nil
	default:
		if m.planApprovalCursor == 2 && len(msg.Runes) > 0 {
			m.planApprovalInput += string(msg.Runes)
			m.updateViewport()
		}
		return m, nil
	}
}

func (m Model) executePlanApproval() (tea.Model, tea.Cmd) {
	m.planApprovalDialog = false
	wd, _ := os.Getwd()

	var modeMsg string
	switch m.planApprovalCursor {
	case 0: // YOLO mode
		if m.ag != nil && m.ag.Checker != nil {
			m.ag.Checker.Mode = permissions.ModeBypass
			m.ag.Checker.PlanFilePath = ""
		}
		modeMsg = "Plan approved. Entered YOLO mode (all operations auto-approved)."
	case 1: // Manually approve
		if m.ag != nil && m.ag.Checker != nil {
			restoreMode := m.prePlanMode
			if restoreMode == "" {
				restoreMode = permissions.ModeDefault
			}
			m.ag.Checker.Mode = restoreMode
			m.ag.Checker.PlanFilePath = ""
		}
		modeMsg = "Plan approved. Each edit will require your confirmation."
	}

	m.chatMessages = append(m.chatMessages, chatMessage{
		role:    "system",
		content: modeMsg,
	})

	// Load the plan and send it as context for the agent to start executing
	planPath := plan_file.GetPlanFilePath(wd)
	planContent, _ := plan_file.LoadPlan(wd)
	planExists := plan_file.PlanExists(wd)
	plan_file.ResetPlanPath()

	executeMsg := prompt.BuildPlanModeExitReminder(planPath, planExists)
	// Mark that Plan Mode has been exited in this session; a reminder can be injected on subsequent re-entry
	m.hasExitedPlanMode = true
	executeMsg += "\n\nUser has approved your plan. You can now start coding."
	if planContent != "" {
		executeMsg += "\n\nApproved Plan:\n" + planContent
	}

	m.updateViewport()
	return m.sendMessage(executeMsg)
}

func (m Model) sendPlanFeedback(feedback string, alsoExit bool) (tea.Model, tea.Cmd) {
	m.planApprovalDialog = false
	m.planApprovalInput = ""

	if alsoExit {
		if m.ag != nil && m.ag.Checker != nil {
			restoreMode := m.prePlanMode
			if restoreMode == "" {
				restoreMode = permissions.ModeDefault
			}
			m.ag.Checker.Mode = restoreMode
			m.ag.Checker.PlanFilePath = ""
		}
		plan_file.ResetPlanPath()
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "system",
			content: "Exiting plan mode with feedback. Edits will require confirmation.",
		})
	}

	m.updateViewport()
	return m.sendMessage(feedback)
}

func (m Model) renderPlanApprovalDialog() string {
	var sb strings.Builder

	header := lipgloss.NewStyle().Foreground(brandPurple).Bold(true).Render(
		" Swiftx has written up a plan and is ready to execute. Would you like to proceed?",
	)
	sb.WriteString(header)
	sb.WriteString("\n\n")

	options := []string{
		"Yes, enter YOLO mode (auto-approve all)",
		"Yes, manually approve edits",
		"Tell Swiftx what to change",
	}

	for i, opt := range options {
		prefix := "   "
		if i == m.planApprovalCursor {
			prefix = lipgloss.NewStyle().Foreground(brandPurple).Render(" > ")
		}
		label := opt
		if i == m.planApprovalCursor {
			label = lipgloss.NewStyle().Bold(true).Render(opt)
		} else {
			label = lipgloss.NewStyle().Foreground(dimText).Render(opt)
		}
		sb.WriteString(prefix)
		fmt.Fprintf(&sb, "%d. %s", i+1, label)
		sb.WriteString("\n")

		if i == 2 {
			inputLine := m.planApprovalInput
			if m.planApprovalCursor == 2 {
				inputLine += "|"
			}
			if inputLine == "|" || inputLine == "" {
				placeholder := lipgloss.NewStyle().Foreground(dimText).Render("Type feedback here...")
				if m.planApprovalCursor == 2 {
					sb.WriteString("      ")
					sb.WriteString(placeholder)
					sb.WriteString("\n")
				}
			} else {
				sb.WriteString("      " + inputLine + "\n")
			}
			hint := lipgloss.NewStyle().Foreground(dimText).Render("      shift+tab to approve with this feedback")
			sb.WriteString(hint)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// handleSandboxDialog handles key interactions in the sandbox mode selection dialog
func (m Model) handleSandboxDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	labels := commands.SandboxModeLabels()
	switch msg.String() {
	case "up", "k":
		if m.sandboxCursor > 0 {
			m.sandboxCursor--
		}
		m.updateViewport()
		return m, nil
	case "down", "j":
		if m.sandboxCursor < len(labels)-1 {
			m.sandboxCursor++
		}
		m.updateViewport()
		return m, nil
	case "enter":
		m.sandboxDialog = false
		mode := commands.SandboxMode(m.sandboxCursor)
		return m.applySandboxMode(mode)
	case "escape":
		m.sandboxDialog = false
		m.updateViewport()
		return m, nil
	}
	return m, nil
}

// applySandboxMode updates the BashTool and permission checker based on the selected mode
func (m Model) applySandboxMode(mode commands.SandboxMode) (tea.Model, tea.Cmd) {
	bashTool, _ := m.registry.Get("Bash").(*tools.BashTool)
	labels := commands.SandboxModeLabels()
	descriptions := commands.SandboxModeDescriptions()

	switch mode {
	case commands.SandboxAutoAllow:
		// Enable sandbox + auto-allow
		sb := sandbox.New()
		if bashTool != nil {
			bashTool.Sandbox = sb
			if m.ag != nil && m.ag.Checker != nil && m.ag.Checker.Sandbox != nil {
				bashTool.SandboxConfig = sandbox.Config{
					AllowWrite:     m.ag.Checker.Sandbox.GetAllowedRoots(),
					DenyWrite:      m.ag.Checker.Sandbox.GetDenyWrite(),
					NetworkEnabled: false,
				}
			}
		}
		if m.ag != nil && m.ag.Checker != nil {
			m.ag.Checker.SandboxEnabled = true
		}
	case commands.SandboxRegular:
		// Enable sandbox but retain regular permission confirmation
		sb := sandbox.New()
		if bashTool != nil {
			bashTool.Sandbox = sb
			if m.ag != nil && m.ag.Checker != nil && m.ag.Checker.Sandbox != nil {
				bashTool.SandboxConfig = sandbox.Config{
					AllowWrite:     m.ag.Checker.Sandbox.GetAllowedRoots(),
					DenyWrite:      m.ag.Checker.Sandbox.GetDenyWrite(),
					NetworkEnabled: false,
				}
			}
		}
		if m.ag != nil && m.ag.Checker != nil {
			m.ag.Checker.SandboxEnabled = false
		}
	case commands.SandboxOff:
		// Disable sandbox
		if bashTool != nil {
			bashTool.Sandbox = nil
			bashTool.SandboxConfig = sandbox.Config{}
		}
		if m.ag != nil && m.ag.Checker != nil {
			m.ag.Checker.SandboxEnabled = false
		}
	}

	msg := fmt.Sprintf("Sandbox mode switched to: %s\n%s", labels[mode], descriptions[mode])
	m.chatMessages = append(m.chatMessages, chatMessage{role: "system", content: msg})
	m.updateViewport()
	return m, nil
}

// renderSandboxDialog renders the sandbox mode selection UI
func (m Model) renderSandboxDialog() string {
	if !m.sandboxDialog {
		return ""
	}
	var sb strings.Builder

	header := lipgloss.NewStyle().Foreground(brandPurple).Bold(true).Render(
		" Select Sandbox Mode",
	)
	sb.WriteString(header)
	sb.WriteString("\n\n")

	labels := commands.SandboxModeLabels()
	descs := commands.SandboxModeDescriptions()

	for i, label := range labels {
		prefix := "   "
		if i == m.sandboxCursor {
			prefix = lipgloss.NewStyle().Foreground(brandPurple).Render(" > ")
		}
		displayLabel := label
		if i == m.sandboxCursor {
			displayLabel = lipgloss.NewStyle().Bold(true).Render(label)
		} else {
			displayLabel = lipgloss.NewStyle().Foreground(dimText).Render(label)
		}
		desc := lipgloss.NewStyle().Foreground(dimText).Render(" — " + descs[i])
		fmt.Fprintf(&sb, "%s%d. %s%s\n", prefix, i+1, displayLabel, desc)
	}
	sb.WriteString("\n")
	return sb.String()
}

func (m Model) handleAskUserDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	multiQuestion := len(m.askUserQuestions) > 1

	// Submit tab handling
	if m.askUserOnSubmit {
		switch msg.String() {
		case "up", "k":
			if m.askUserSubmitIdx > 0 {
				m.askUserSubmitIdx--
			}
			m.updateViewport()
			return m, nil
		case "down", "j":
			if m.askUserSubmitIdx < 1 {
				m.askUserSubmitIdx++
			}
			m.updateViewport()
			return m, nil
		case "left", "shift+tab":
			m.askUserOnSubmit = false
			m.askUserQIdx = len(m.askUserQuestions) - 1
			m.updateViewport()
			return m, nil
		case "enter":
			if m.askUserSubmitIdx == 0 {
				return m.submitAllAnswers()
			}
			return m.cancelAskUser()
		case "escape":
			return m.cancelAskUser()
		}
		return m, nil
	}

	q := m.askUserQuestions[m.askUserQIdx]
	optCount := len(q.Options) + 1
	cursor := m.askUserCursors[m.askUserQIdx]

	switch msg.String() {
	case "up", "k":
		if cursor > 0 {
			m.askUserCursors[m.askUserQIdx]--
		}
		m.updateViewport()
		return m, nil
	case "down", "j":
		if cursor < optCount-1 {
			m.askUserCursors[m.askUserQIdx]++
		}
		m.updateViewport()
		return m, nil
	case "left", "shift+tab":
		if multiQuestion && m.askUserQIdx > 0 {
			m.askUserQIdx--
			m.updateViewport()
		}
		return m, nil
	case "right", "tab":
		if multiQuestion {
			if m.askUserQIdx < len(m.askUserQuestions)-1 {
				m.askUserQIdx++
			} else {
				m.askUserOnSubmit = true
				m.askUserSubmitIdx = 0
			}
			m.updateViewport()
		}
		return m, nil
	case " ":
		if q.MultiSelect && cursor < len(q.Options) {
			sel := m.askUserSelected[m.askUserQIdx]
			sel[cursor] = !sel[cursor]
			m.updateViewport()
			return m, nil
		}
	case "enter":
		m.saveCurrentAnswer()

		if !multiQuestion && !q.MultiSelect {
			return m.submitAllAnswers()
		}

		if m.askUserQIdx < len(m.askUserQuestions)-1 {
			m.askUserQIdx++
		} else {
			m.askUserOnSubmit = true
			m.askUserSubmitIdx = 0
		}
		m.updateViewport()
		return m, nil
	case "backspace":
		if cursor == len(q.Options) && len(m.askUserOther[m.askUserQIdx]) > 0 {
			s := m.askUserOther[m.askUserQIdx]
			_, size := utf8.DecodeLastRuneInString(s)
			m.askUserOther[m.askUserQIdx] = s[:len(s)-size]
			m.updateViewport()
		}
		return m, nil
	case "escape":
		return m.cancelAskUser()
	default:
		if cursor == len(q.Options) && len(msg.Runes) > 0 {
			m.askUserOther[m.askUserQIdx] += string(msg.Runes)
			m.updateViewport()
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) saveCurrentAnswer() {
	q := m.askUserQuestions[m.askUserQIdx]
	cursor := m.askUserCursors[m.askUserQIdx]

	if cursor == len(q.Options) {
		other := m.askUserOther[m.askUserQIdx]
		if other == "" {
			other = "Other"
		}
		m.askUserAnswered[m.askUserQIdx] = other
	} else if q.MultiSelect {
		var selected []string
		for i, opt := range q.Options {
			if m.askUserSelected[m.askUserQIdx][i] {
				selected = append(selected, opt.Label)
			}
		}
		if len(selected) == 0 {
			selected = append(selected, q.Options[cursor].Label)
		}
		m.askUserAnswered[m.askUserQIdx] = strings.Join(selected, ", ")
	} else {
		m.askUserAnswered[m.askUserQIdx] = q.Options[cursor].Label
	}
}

func (m *Model) collectAskUserAnswers() map[string]string {
	result := make(map[string]string)
	for idx, answer := range m.askUserAnswered {
		if idx < len(m.askUserQuestions) {
			result[m.askUserQuestions[idx].Text] = answer
		}
	}
	return result
}

func (m *Model) submitAllAnswers() (tea.Model, tea.Cmd) {
	m.askUserDialog = false
	m.askUserRespCh <- tools.QuestionResponse{Answers: m.collectAskUserAnswers()}
	m.updateViewport()
	return m, tea.Batch(m.listenForAgentEvents(), m.listenForAskUser())
}

func (m *Model) cancelAskUser() (tea.Model, tea.Cmd) {
	m.askUserDialog = false
	m.askUserRespCh <- tools.QuestionResponse{Answers: map[string]string{"_declined": "true"}}
	m.updateViewport()
	return m, tea.Batch(m.listenForAgentEvents(), m.listenForAskUser())
}

func (m Model) renderAskUserDialog() string {
	if !m.askUserDialog || len(m.askUserQuestions) == 0 {
		return ""
	}
	var sb strings.Builder
	multiQuestion := len(m.askUserQuestions) > 1

	// Navigation bar (only for multi-question)
	if multiQuestion {
		sb.WriteString(m.renderQuestionNavBar())
		sb.WriteString("\n\n")
	}

	if m.askUserOnSubmit {
		sb.WriteString(m.renderSubmitView())
	} else {
		sb.WriteString(m.renderQuestionView())
	}

	// Bottom hint
	if multiQuestion && !m.askUserOnSubmit {
		hint := lipgloss.NewStyle().Foreground(dimText).Render("      ← → navigate questions · enter to confirm")
		sb.WriteString(hint)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func (m Model) renderQuestionNavBar() string {
	var sb strings.Builder
	activeTab := lipgloss.NewStyle().Background(lipgloss.Color("99")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1)
	inactiveTab := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Padding(0, 1)
	dimArrow := lipgloss.NewStyle().Foreground(dimText)
	brightArrow := lipgloss.NewStyle().Foreground(brandPurple).Bold(true)

	// Left arrow
	if m.askUserQIdx == 0 && !m.askUserOnSubmit {
		sb.WriteString(dimArrow.Render(" ←"))
	} else {
		sb.WriteString(brightArrow.Render(" ←"))
	}

	// Question tabs
	for i, q := range m.askUserQuestions {
		header := q.Header
		if header == "" {
			header = fmt.Sprintf("Q%d", i+1)
		}
		_, answered := m.askUserAnswered[i]
		check := "☐"
		if answered {
			check = "☑"
		}
		label := header + " " + check

		if !m.askUserOnSubmit && i == m.askUserQIdx {
			sb.WriteString(activeTab.Render(label))
		} else {
			sb.WriteString(inactiveTab.Render(label))
		}
	}

	// Submit tab
	submitLabel := "✓ Submit"
	if m.askUserOnSubmit {
		sb.WriteString(activeTab.Render(submitLabel))
	} else {
		sb.WriteString(inactiveTab.Render(submitLabel))
	}

	// Right arrow
	if m.askUserOnSubmit {
		sb.WriteString(dimArrow.Render(" →"))
	} else {
		sb.WriteString(brightArrow.Render(" →"))
	}

	return sb.String()
}

func (m Model) askUserMaxLines() int {
	maxLines := 0
	for _, q := range m.askUserQuestions {
		lines := 2 + len(q.Options) + 1 // header + blank + options + Other
		if q.MultiSelect {
			lines++ // "space to toggle" hint
		}
		if lines > maxLines {
			maxLines = lines
		}
	}
	return maxLines
}

func (m Model) renderQuestionView() string {
	var sb strings.Builder
	q := m.askUserQuestions[m.askUserQIdx]
	cursor := m.askUserCursors[m.askUserQIdx]
	lines := 0

	header := lipgloss.NewStyle().Foreground(brandPurple).Bold(true).Render(" " + q.Text)
	sb.WriteString(header)
	sb.WriteString("\n\n")
	lines += 2

	for i, opt := range q.Options {
		prefix := "   "
		if i == cursor {
			prefix = lipgloss.NewStyle().Foreground(brandPurple).Render(" > ")
		}
		if q.MultiSelect {
			check := "○"
			if m.askUserSelected[m.askUserQIdx][i] {
				check = "●"
			}
			prefix += check + " "
		}
		label := opt.Label
		if i == cursor {
			label = lipgloss.NewStyle().Bold(true).Render(opt.Label)
		}
		desc := lipgloss.NewStyle().Foreground(dimText).Render(" — " + opt.Description)
		fmt.Fprintf(&sb, "%s%s%s\n", prefix, label, desc)
		lines++
	}

	// "Other" option
	otherIdx := len(q.Options)
	prefix := "   "
	if cursor == otherIdx {
		prefix = lipgloss.NewStyle().Foreground(brandPurple).Render(" > ")
	}
	otherLabel := "Other"
	if cursor == otherIdx {
		otherLabel = lipgloss.NewStyle().Bold(true).Render("Other")
	} else {
		otherLabel = lipgloss.NewStyle().Foreground(dimText).Render("Other")
	}
	fmt.Fprintf(&sb, "%s%s", prefix, otherLabel)
	if cursor == otherIdx {
		input := m.askUserOther[m.askUserQIdx] + "|"
		sb.WriteString(": ")
		sb.WriteString(input)
	}
	sb.WriteString("\n")
	lines++

	if q.MultiSelect {
		sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("      space to toggle, enter to confirm"))
		sb.WriteString("\n")
		lines++
	}

	// Pad to fixed height so switching questions doesn't cause layout shift
	if len(m.askUserQuestions) > 1 {
		target := m.askUserMaxLines()
		for lines < target {
			sb.WriteString("\n")
			lines++
		}
	}

	return sb.String()
}

func (m Model) renderSubmitView() string {
	var sb strings.Builder
	lines := 0

	header := lipgloss.NewStyle().Foreground(brandPurple).Bold(true).Render(" Review your answers:")
	sb.WriteString(header)
	sb.WriteString("\n\n")
	lines += 2

	for i, q := range m.askUserQuestions {
		label := q.Header
		if label == "" {
			label = fmt.Sprintf("Q%d", i+1)
		}
		answer, ok := m.askUserAnswered[i]
		if ok {
			fmt.Fprintf(&sb, "   %s: %s\n", label, answer)
		} else {
			dim := lipgloss.NewStyle().Foreground(dimText).Render(fmt.Sprintf("   %s: (not answered)", label))
			sb.WriteString(dim + "\n")
		}
		lines++
	}
	sb.WriteString("\n")
	lines++

	// Submit / Cancel options
	for i, opt := range []string{"Submit answers", "Cancel"} {
		if i == m.askUserSubmitIdx {
			prefix := lipgloss.NewStyle().Foreground(brandPurple).Render(" > ")
			label := lipgloss.NewStyle().Bold(true).Render(opt)
			sb.WriteString(prefix)
			sb.WriteString(label)
			sb.WriteString("\n")
		} else {
			sb.WriteString("   ")
			sb.WriteString(opt)
			sb.WriteString("\n")
		}
		lines++
	}

	// Pad to match question view height
	target := m.askUserMaxLines()
	for lines < target {
		sb.WriteString("\n")
		lines++
	}

	return sb.String()
}

func expandAtRefs(text string) string {
	re := regexp.MustCompile(`@([\w./_-]+)`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		path := strings.TrimPrefix(match, "@")
		path = strings.TrimSuffix(path, "/")
		data, err := os.ReadFile(path)
		if err != nil {
			return match
		}
		content := string(data)
		if len(content) > 10000 {
			content = content[:10000] + "\n… (truncated)"
		}
		return fmt.Sprintf("[File: %s]\n```\n%s\n```", path, content)
	})
}

func (m Model) sendMessage(text string) (tea.Model, tea.Cmd) {
	m.refreshSkillsIfNeeded()
	wd, _ := os.Getwd()
	history.Append(wd, text)
	m.historyEntries = append(m.historyEntries, text)
	m.historyIndex = 0
	m.historyDraft = ""
	session.SaveMessage(wd, m.sessionID, session.Message{Role: "user", Content: text, Ts: time.Now().Unix()})

	m.streaming = true
	m.thinking = true
	m.thinkingStart = time.Now()
	m.thinkingDone = 0
	m.thinkingVerb = randomVerb()
	m.atMenuOpen = false
	m.atMatches = nil
	m.textarea.Reset()
	m.textarea.SetHeight(1)

	expanded := expandAtRefs(text)

	m.chatMessages = append(m.chatMessages, chatMessage{role: "user", content: text})
	userLine := promptStyle.Render("> ") + lipgloss.NewStyle().Foreground(brightText).Bold(true).Render(text)
	m.committedUpTo = len(m.chatMessages)
	m.conversation.AddUserMessage(expanded)

	if m.mcpInstructions != "" && !m.mcpInstructionsOK {
		m.conversation.AddSystemReminder(m.mcpInstructions)
		m.mcpInstructionsOK = true
	}

	prefetchCh := m.prefetchRelevantMemories(expanded)

	m.streamBuf = ""
	m.toolBlocks = nil
	m.userScrolled = false

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelStream = cancel

	conv := m.conversation
	ag := m.ag
	// Non-blocking memory recall: prefetchCh is passed to the agent and injected after tool execution
	ag.MemoryRecallCh = prefetchCh
	startAgentCmd := func() tea.Msg {
		return agentReadyMsg{ch: ag.Run(ctx, conv)}
	}

	m.updateViewport()
	return m, tea.Batch(tea.Println(userLine), startAgentCmd, m.listenForAskUser(), m.listenForSubAgentProgress(), m.spinner.Tick)
}

func (m Model) sendPromptCommand(displayText, prompt string) (tea.Model, tea.Cmd) {
	wd, _ := os.Getwd()
	history.Append(wd, displayText)
	m.historyEntries = append(m.historyEntries, displayText)
	m.historyIndex = 0
	m.historyDraft = ""
	session.SaveMessage(wd, m.sessionID, session.Message{Role: "user", Content: displayText, Ts: time.Now().Unix()})

	m.streaming = true
	m.thinking = true
	m.thinkingStart = time.Now()
	m.thinkingDone = 0
	m.thinkingVerb = randomVerb()
	m.atMenuOpen = false
	m.atMatches = nil
	m.textarea.Reset()
	m.textarea.SetHeight(1)

	m.chatMessages = append(m.chatMessages, chatMessage{role: "user", content: displayText})
	userLine := promptStyle.Render("> ") + lipgloss.NewStyle().Foreground(brightText).Bold(true).Render(displayText)
	m.committedUpTo = len(m.chatMessages)
	m.conversation.AddUserMessage(prompt)

	prefetchCh := m.prefetchRelevantMemories(prompt)

	m.streamBuf = ""
	m.toolBlocks = nil
	m.userScrolled = false

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelStream = cancel

	conv := m.conversation
	ag := m.ag
	ag.MemoryRecallCh = prefetchCh
	startAgentCmd := func() tea.Msg {
		return agentReadyMsg{ch: ag.Run(ctx, conv)}
	}

	m.updateViewport()
	return m, tea.Batch(tea.Println(userLine), startAgentCmd, m.listenForAskUser(), m.listenForSubAgentProgress(), m.spinner.Tick)
}

type askUserMsg struct {
	req tools.AskUserRequest
}

type subAgentProgressMsg struct {
	progress subagent.SubAgentProgress
}

func (m *Model) drainSubAgentProgress() {
	for {
		select {
		case p := <-m.subAgentProgressCh:
			if p.Done {
				if m.activeSubAgent != nil {
					m.activeSubAgent.done = true
					m.activeSubAgent.toolCount = p.ToolCount
					m.activeSubAgent.totalTime = p.TotalTime
				}
			} else {
				if m.activeSubAgent == nil || m.activeSubAgent.done {
					m.activeSubAgent = &subAgentBlock{
						desc:      p.AgentDesc,
						agentType: p.AgentType,
					}
				}
				m.activeSubAgent.toolUses = append(m.activeSubAgent.toolUses, toolBlockInfo{
					toolName: p.ToolName,
					args:     p.ToolArgs,
					elapsed:  p.Elapsed,
					isError:  p.IsError,
				})
			}
		default:
			return
		}
	}
}

func (m Model) listenForSubAgentProgress() tea.Cmd {
	ch := m.subAgentProgressCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		p := <-ch
		return subAgentProgressMsg{progress: p}
	}
}

func (m Model) listenForAskUser() tea.Cmd {
	ch := m.askUserCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req := <-ch
		return askUserMsg{req: req}
	}
}

func (m Model) listenForAgentEvents() tea.Cmd {
	ch := m.agentCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentDoneMsg{ch: ch}
		}
		return agentEventMsg{event: ev, ch: ch}
	}
}

func (m Model) pollMailbox() tea.Cmd {
	if m.teamMgr == nil {
		return nil
	}
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return mailboxPollMsg{}
	})
}

func (m Model) handleAgentEvent(ev agent.AgentEvent) (tea.Model, tea.Cmd) {
	switch e := ev.(type) {
	case agent.StreamText:
		m.streamBuf += e.Text
		m.updateViewport()

	case agent.ToolUseEvent:
		if e.Args != nil {
			found := false
			for i := range m.toolBlocks {
				if m.toolBlocks[i].toolName == e.ToolName && m.toolBlocks[i].args == nil {
					m.toolBlocks[i].args = e.Args
					found = true
					break
				}
			}
			if !found {
				m.toolBlocks = append(m.toolBlocks, toolBlockInfo{
					toolName: e.ToolName,
					args:     e.Args,
					loading:  true,
				})
			}
		} else {
			m.toolBlocks = append(m.toolBlocks, toolBlockInfo{
				toolName: e.ToolName,
				loading:  true,
			})
		}
		m.updateViewport()

	case agent.ToolResultEvent:
		var emitted *toolBlockInfo
		for i := range m.toolBlocks {
			if m.toolBlocks[i].toolName == e.ToolName && m.toolBlocks[i].loading {
				m.toolBlocks[i].output = e.Output
				m.toolBlocks[i].isError = e.IsError
				m.toolBlocks[i].elapsed = e.Elapsed.Seconds()
				m.toolBlocks[i].loading = false
				m.toolBlocks[i].collapsed = true
				// Agent tools defer to TurnComplete so we can merge sub-agent progress.
				if m.toolBlocks[i].toolName != "Agent" {
					tb := m.toolBlocks[i]
					emitted = &tb
					m.toolBlocks = append(m.toolBlocks[:i], m.toolBlocks[i+1:]...)
				}
				break
			}
		}
		if emitted != nil {
			// Flush pending assistant text first so order matches Swiftx's
			// per-block stream: text → tool result → text → tool result.
			if m.streamBuf != "" {
				m.chatMessages = append(m.chatMessages, chatMessage{
					role:    "assistant",
					content: m.streamBuf,
				})
				m.streamBuf = ""
			}
			m.chatMessages = append(m.chatMessages, chatMessage{
				role:      "tool_visible",
				content:   renderToolBlockText(*emitted),
				toolGroup: []toolBlockInfo{*emitted},
			})
			commitText := m.renderMessagesRange(m.committedUpTo, len(m.chatMessages))
			m.committedUpTo = len(m.chatMessages)
			m.updateViewport()
			if commitText != "" {
				return m, tea.Batch(tea.Println(commitText), m.listenForAgentEvents())
			}
		}
		m.updateViewport()

	case agent.TurnComplete:
		if m.streamBuf != "" {
			m.chatMessages = append(m.chatMessages, chatMessage{role: "assistant", content: m.streamBuf})
			m.streamBuf = ""
		}
		// Drain any buffered sub-agent progress events
		m.drainSubAgentProgress()
		if len(m.toolBlocks) > 0 {
			var nonAgentTools []toolBlockInfo
			for _, tb := range m.toolBlocks {
				if tb.toolName == "Agent" && m.activeSubAgent != nil && len(m.activeSubAgent.toolUses) > 0 {
					// Finalize sub-agent block using collected progress
					sab := *m.activeSubAgent
					if !sab.done {
						sab.done = true
						sab.toolCount = len(sab.toolUses)
						var total float64
						for _, tu := range sab.toolUses {
							total += tu.elapsed
						}
						sab.totalTime = total
					}
					m.chatMessages = append(m.chatMessages, chatMessage{
						role:          "sub_agent",
						subAgentBlock: &sab,
						expanded:      false,
					})
					m.activeSubAgent = nil
				} else if tb.toolName == "Agent" {
					// Agent tool ran but no progress collected — extract info from result
					desc := ""
					if d, ok := tb.args["description"].(string); ok {
						desc = d
					}
					agentType := "general-purpose"
					if at, ok := tb.args["subagent_type"].(string); ok {
						agentType = at
					}
					sab := &subAgentBlock{
						desc:      desc,
						agentType: agentType,
						done:      true,
						totalTime: tb.elapsed,
						toolCount: 0,
					}
					m.chatMessages = append(m.chatMessages, chatMessage{
						role:          "sub_agent",
						subAgentBlock: sab,
						expanded:      false,
					})
					m.activeSubAgent = nil
				} else {
					nonAgentTools = append(nonAgentTools, tb)
				}
			}
			// Classify non-agent tools: visible (write/command) vs collapsed (read)
			var visibleTools, collapsedTools []toolBlockInfo
			for _, tb := range nonAgentTools {
				if isCollapsibleTool(tb.toolName) {
					collapsedTools = append(collapsedTools, tb)
				} else {
					visibleTools = append(visibleTools, tb)
				}
			}
			// Visible tools: show each as individual line
			for _, tb := range visibleTools {
				m.chatMessages = append(m.chatMessages, chatMessage{
					role:      "tool_visible",
					content:   renderToolBlockText(tb),
					toolGroup: []toolBlockInfo{tb},
				})
			}
			// Collapsed reads: hidden by default, shown on ctrl+o
			if len(collapsedTools) > 0 {
				m.chatMessages = append(m.chatMessages, chatMessage{
					role:      "tool_collapsed",
					toolGroup: collapsedTools,
					expanded:  false,
				})
			}
		}
		m.toolBlocks = nil
		m.activeSubAgent = nil
		m.updateViewport()

	case agent.UsageEvent:
		m.totalInput = e.InputTokens
		m.totalOutput = e.OutputTokens

	case agent.PermissionRequestEvent:
		m.permDialog = true
		m.permCursor = 0
		m.permToolName = e.ToolName
		m.permDesc = e.Desc
		m.permRespCh = e.ResponseCh
		m.updateViewport()
		return m, nil

	case agent.CompactEvent:
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "system",
			content: "⟳ " + e.Message,
		})
		m.updateViewport()

	case agent.RetryEvent:
		msg := "↻ Retrying: " + e.Reason
		if e.Wait > 0 {
			msg += fmt.Sprintf(" (waiting %s)", e.Wait)
		}
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "system",
			content: msg,
		})
		m.updateViewport()

	case agent.ErrorEvent:
		// Preserve any streamed text already emitted before the error
		if m.streamBuf != "" {
			m.chatMessages = append(m.chatMessages, chatMessage{role: "assistant", content: m.streamBuf})
			m.streamBuf = ""
		}
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "error",
			content: e.Message,
		})
		commitText := m.renderMessagesRange(m.committedUpTo, len(m.chatMessages))
		m.committedUpTo = len(m.chatMessages)
		m.finishStreaming()
		if commitText != "" {
			return m, tea.Println(commitText)
		}
		return m, nil

	case agent.LoopComplete:
		totalTime := time.Since(m.thinkingStart).Seconds()
		if m.streamBuf != "" {
			// Assistant messages are persisted by the main loop when entering the
			// conversation history; here we only render them on screen
			m.chatMessages = append(m.chatMessages, chatMessage{role: "assistant", content: m.streamBuf})
			m.streamBuf = ""
		}
		m.chatMessages = append(m.chatMessages, chatMessage{
			role:    "thinking",
			content: fmt.Sprintf("✻ %s for %.1fs", m.pastTense(m.thinkingVerb), totalTime),
		})
		commitText := m.renderMessagesRange(m.committedUpTo, len(m.chatMessages))
		m.committedUpTo = len(m.chatMessages)
		m.thinkingDone = 0
		m.finishStreaming()
		if m.ag != nil && m.ag.Checker != nil && m.ag.Checker.Mode == permissions.ModePlan {
			m.planApprovalDialog = true
			m.planApprovalCursor = 0
			m.planApprovalInput = ""
			m.updateViewport()
		}
		pollCmd := m.pollMailbox()
		if commitText != "" {
			return m, tea.Batch(tea.Println(commitText), pollCmd)
		}
		if pollCmd != nil {
			return m, pollCmd
		}
		return m, nil
	}

	return m, m.listenForAgentEvents()
}

func renderToolBlockText(tb toolBlockInfo) string {
	title := toolTitle(tb.toolName, tb.args)
	if tb.isError {
		return fmt.Sprintf("✗ %s (%.1fs)", title, tb.elapsed)
	}
	return fmt.Sprintf("✓ %s (%.1fs)", title, tb.elapsed)
}

func renderSubAgentBlock(sab *subAgentBlock, expanded bool) string {
	var sb strings.Builder

	agentLabel := cases.Title(language.AmericanEnglish).String(sab.agentType)
	if agentLabel == "" {
		agentLabel = "Agent"
	}
	header := lipgloss.NewStyle().Foreground(brandPurple).Bold(true).Render(
		fmt.Sprintf("● %s(%s)", agentLabel, sab.desc))
	sb.WriteString(header)
	sb.WriteString("\n")

	if sab.done {
		if expanded {
			for _, tu := range sab.toolUses {
				title := toolTitle(tu.toolName, tu.args)
				line := fmt.Sprintf("     %s (%.1fs)", title, tu.elapsed)
				if tu.isError {
					sb.WriteString(toolErrorStyle.Render(line))
				} else {
					sb.WriteString(toolDoneStyle.Render(line))
				}
				sb.WriteString("\n")
			}
		} else {
			summary := fmt.Sprintf("  ⎿  Done (%d tool uses · %.1fs)", sab.toolCount, sab.totalTime)
			sb.WriteString(toolDoneStyle.Render(summary))
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("  (ctrl+o to expand)"))
			sb.WriteString("\n")
		}
	} else {
		n := len(sab.toolUses)
		if n > 0 {
			last := sab.toolUses[n-1]
			lastTitle := toolTitle(last.toolName, last.args)
			sb.WriteString(toolDoneStyle.Render(fmt.Sprintf("  ⎿  %s (%.1fs)", lastTitle, last.elapsed)))
			sb.WriteString("\n")
		}
		if n > 1 {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render(
				fmt.Sprintf("     … +%d tool uses (ctrl+o to expand)", n-1)))
			sb.WriteString("\n")
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("     Running…"))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

func isCollapsibleTool(name string) bool {
	switch name {
	case "ReadFile", "Glob", "Grep", "ToolSearch":
		return true
	}
	return false
}

// isDiffTool reports whether the tool's output is a line-numbered diff produced by BuildDiff.
func isDiffTool(name string) bool {
	return name == "EditFile"
}

// renderDiffLines renders the line-numbered diff text produced by tools.BuildDiff()
// into colored lines: lines starting with "+ " are green, lines starting with
// "- " are red, and all others (context/summary lines) use toolDetailStyle.
func renderDiffLines(output string) string {
	lines := strings.Split(output, "\n")
	rendered := make([]string, len(lines))
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+ "):
			rendered[i] = diffAddStyle.Render(line)
		case strings.HasPrefix(line, "- "):
			rendered[i] = diffRemoveStyle.Render(line)
		default:
			rendered[i] = toolDetailStyle.Render(line)
		}
	}
	return strings.Join(rendered, "\n")
}

// appendEditDiff appends the EditFile diff body (if any) after the rendered tool title line.
func appendEditDiff(sb *strings.Builder, toolGroup []toolBlockInfo) {
	if len(toolGroup) != 1 {
		return
	}
	tb := toolGroup[0]
	if !isDiffTool(tb.toolName) || tb.output == "" {
		return
	}
	sb.WriteString(renderDiffLines(tb.output))
	sb.WriteString("\n")
}

func renderToolGroupSummary(tools []toolBlockInfo) string {
	var totalElapsed float64
	errors := 0
	for _, tb := range tools {
		totalElapsed += tb.elapsed
		if tb.isError {
			errors++
		}
	}
	n := len(tools)
	if errors > 0 {
		return fmt.Sprintf("● Done (%d tool uses · %d errors · %.1fs)", n, errors, totalElapsed)
	}
	return fmt.Sprintf("● Done (%d tool uses · %.1fs)", n, totalElapsed)
}

func (m *Model) calcViewportHeight(availableHeight int) int {
	contentLines := max(m.viewport.TotalLineCount(), 1)
	if contentLines > availableHeight {
		return availableHeight
	}
	return contentLines
}

func (m *Model) updateViewport() {
	if !m.ready {
		return
	}
	content := m.renderChatContent()
	m.viewport.SetContent(content)

	statusHeight, sepHeight := 1, 1
	inputHeight := m.textarea.Height() + 1
	available := max(m.height-statusHeight-sepHeight-inputHeight-1, 1)
	m.viewport.Height = m.calcViewportHeight(available)

	if !m.userScrolled {
		m.viewport.GotoBottom()
	}
}

// ── View ──

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	switch m.state {
	case stateProviderSelect:
		return m.renderProviderSelectView()
	case stateChat:
		return m.renderChatView()
	case stateResume:
		return m.renderResumeView()
	}
	return ""
}

func (m Model) renderProviderSelectView() string {
	var sb strings.Builder

	sb.WriteString(m.renderBanner())
	sb.WriteString("\n\n")

	sb.WriteString(selectLabelStyle.Render("Select a Provider"))
	sb.WriteString("\n\n")

	for i, p := range m.providers {
		if i == m.providerCursor {
			sb.WriteString(selectedItemStyle.Render(fmt.Sprintf("  > %s  [%s]", p.Name, p.Model)))
		} else {
			sb.WriteString(normalItemStyle.Render(fmt.Sprintf("    %s  [%s]", p.Name, p.Model)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderTeammateTree() string {
	if m.teamMgr == nil {
		return ""
	}
	progressList := m.teamMgr.GetAllTeammateProgress()
	if len(progressList) == 0 {
		return ""
	}

	cyanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00d7ff"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffff00"))

	var sb strings.Builder
	sb.WriteString("\n")

	// Leader line
	sb.WriteString("  ┌─ ")
	sb.WriteString(cyanStyle.Render("team-lead"))
	sb.WriteString(": ")
	sb.WriteString(dimStyle.Render(m.thinkingVerb + "…"))
	if m.totalInput+m.totalOutput > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf(" · %s tokens", teams.FormatTokens(int64(m.totalInput+m.totalOutput)))))
	}
	sb.WriteString("\n")

	// Teammate lines
	for i, p := range progressList {
		isLast := i == len(progressList)-1
		connector := "  ├─ "
		if isLast {
			connector = "  └─ "
		}

		sb.WriteString(connector)
		sb.WriteString(cyanStyle.Render("@" + p.Name))
		sb.WriteString(": ")

		status := p.GetStatus()
		switch status {
		case "completed":
			sb.WriteString(greenStyle.Render("completed"))
		case "failed":
			sb.WriteString(redStyle.Render("failed"))
		case "stopped":
			sb.WriteString(yellowStyle.Render("stopped"))
		case "idle":
			sb.WriteString(dimStyle.Render("idle"))
		default:
			sb.WriteString(dimStyle.Render(p.ActivitySummary() + "..."))
		}

		stats := fmt.Sprintf(" · %d tools · %s tokens", p.GetToolUseCount(), teams.FormatTokens(p.GetTokenCount()))
		sb.WriteString(dimStyle.Render(stats))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderChatView() string {
	var sb strings.Builder

	hasActiveContent := len(m.chatMessages) > m.committedUpTo || m.streaming

	if hasActiveContent {
		bottomLines := 4
		if m.permDialog {
			bottomLines += 3
		}
		vpH := max(m.height-bottomLines, 1)
		m.viewport.Height = m.calcViewportHeight(vpH)
		sb.WriteString(m.viewport.View())
		sb.WriteString("\n")
	}

	if m.permDialog {
		sb.WriteString(m.renderPermDialog())
	}
	if m.planApprovalDialog {
		sb.WriteString(m.renderPlanApprovalDialog())
	}
	if m.askUserDialog {
		sb.WriteString(m.renderAskUserDialog())
	}
	if m.rewindDialog {
		sb.WriteString(m.renderRewindDialog())
	}
	if m.sandboxDialog {
		sb.WriteString(m.renderSandboxDialog())
	}
	sb.WriteString(m.renderSeparator())
	sb.WriteString("\n")
	if m.planApprovalDialog {
		// Hide input when plan approval dialog is active
		sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("  Select an option above..."))
	} else {
		sb.WriteString(promptStyle.Render("> "))
		sb.WriteString(m.textarea.View())
	}
	sb.WriteString("\n")
	sb.WriteString(m.renderSeparator())
	sb.WriteString("\n")
	if m.slashMenuOpen && len(m.slashMatches) > 0 {
		sb.WriteString(m.renderSlashMenu())
	}
	if m.atMenuOpen && len(m.atMatches) > 0 {
		sb.WriteString(m.renderAtMenu())
	}
	sb.WriteString(m.renderStatusBar())
	return sb.String()
}

func (m Model) renderBanner() string {
	cat := bannerStyle.Render(` /\_/\    `) + bannerDimStyle.Render("Swiftx") + "\n" +
		bannerStyle.Render(`( o o )   `) + bannerDimStyle.Render(m.getModelName()) + "\n" +
		bannerStyle.Render(` >   <    `) + bannerDimStyle.Render(m.getWorkDir())
	return cat
}

func (m Model) renderSeparator() string {
	line := strings.Repeat("─", m.width)
	return separatorStyle.Render(line)
}

func (m Model) renderStatusBar() string {
	left := "  default"
	if m.ag != nil && m.ag.Checker != nil && m.ag.Checker.Mode != permissions.ModeDefault {
		name, _ := permissionModeInfo(m.ag.Checker.Mode)
		var modeColor lipgloss.Color
		switch m.ag.Checker.Mode {
		case permissions.ModeAcceptEdits:
			modeColor = greenText
		case permissions.ModePlan:
			modeColor = yellowText
		case permissions.ModeBypass:
			modeColor = redText
		default:
			modeColor = dimText
		}
		modeStr := lipgloss.NewStyle().Foreground(modeColor).Render(
			fmt.Sprintf("%s on", name),
		)
		hint := lipgloss.NewStyle().Foreground(dimText).Render(" (shift+tab to cycle)")
		left = statusBarStyle.Render("  ") + modeStr + hint
	} else {
		left = statusBarStyle.Render(left)
	}

	right := ""
	if m.teamMgr != nil {
		activeCount := 0
		for _, p := range m.teamMgr.GetAllTeammateProgress() {
			if p.GetStatus() == "running" {
				activeCount++
			}
		}
		if activeCount > 0 {
			label := fmt.Sprintf("● %d teammate", activeCount)
			if activeCount > 1 {
				label += "s"
			}
			right += lipgloss.NewStyle().Foreground(cyanText).Render(label + " ")
		}
	}
	if m.mcpConnecting {
		right += lipgloss.NewStyle().Foreground(yellowText).Render("MCP connecting… ")
	}
	if m.selectedProvider != nil {
		right += statusItemStyle.Render(m.selectedProvider.Model)
	}

	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(right)-2, 0)
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) DumpHistory() string {
	if len(m.chatMessages) == 0 {
		return ""
	}
	var sb strings.Builder

	sb.WriteString(m.renderBanner())
	sb.WriteString("\n\n")

	for _, msg := range m.chatMessages {
		switch msg.role {
		case "user":
			sb.WriteString(promptStyle.Render("> "))
			sb.WriteString(lipgloss.NewStyle().Foreground(brightText).Bold(true).Render(msg.content))
			sb.WriteString("\n\n")

		case "assistant":
			sb.WriteString(aiMarkerStyle.Render("● "))
			rendered := m.renderMarkdown(msg.content)
			indented := indentBlock(rendered, "  ")
			sb.WriteString(strings.TrimLeft(indented, " "))
			sb.WriteString("\n\n")

		case "tool", "tool_visible":
			sb.WriteString("  ")
			if strings.HasPrefix(msg.content, "✗") {
				sb.WriteString(toolErrorStyle.Render(msg.content))
			} else {
				sb.WriteString(toolDoneStyle.Render(msg.content))
			}
			sb.WriteString("\n")
			appendEditDiff(&sb, msg.toolGroup)

		case "sub_agent":
			if msg.subAgentBlock != nil {
				sb.WriteString(renderSubAgentBlock(msg.subAgentBlock, false))
			}

		case "system":
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(msg.content))
			sb.WriteString("\n\n")

		case "error":
			sb.WriteString(errorStyle.Render("✗ " + msg.content))
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

func (m Model) renderChatContent() string {
	var sb strings.Builder

	for _, msg := range m.chatMessages[m.committedUpTo:] {
		switch msg.role {
		case "user":
			sb.WriteString(promptStyle.Render("> "))
			sb.WriteString(lipgloss.NewStyle().Foreground(brightText).Bold(true).Render(msg.content))
			sb.WriteString("\n\n")

		case "assistant":
			sb.WriteString(aiMarkerStyle.Render("● "))
			rendered := m.renderMarkdown(msg.content)
			indented := indentBlock(rendered, "  ")
			sb.WriteString(strings.TrimLeft(indented, " "))
			sb.WriteString("\n\n")

		case "tool", "tool_visible":
			sb.WriteString("  ")
			if strings.HasPrefix(msg.content, "✗") {
				sb.WriteString(toolErrorStyle.Render(msg.content))
			} else {
				sb.WriteString(toolDoneStyle.Render(msg.content))
			}
			sb.WriteString("\n")
			appendEditDiff(&sb, msg.toolGroup)

		case "tool_collapsed":
			if msg.expanded {
				for _, tb := range msg.toolGroup {
					sb.WriteString("  ")
					text := renderToolBlockText(tb)
					if tb.isError {
						sb.WriteString(toolErrorStyle.Render(text))
					} else {
						sb.WriteString(toolDoneStyle.Render(text))
					}
					sb.WriteString("\n")
				}
			}
			// Hidden when collapsed — no output

		case "tool_group":
			if msg.expanded {
				for _, tb := range msg.toolGroup {
					sb.WriteString("  ")
					text := renderToolBlockText(tb)
					if tb.isError {
						sb.WriteString(toolErrorStyle.Render(text))
					} else {
						sb.WriteString(toolDoneStyle.Render(text))
					}
					sb.WriteString("\n")
				}
			} else {
				summary := renderToolGroupSummary(msg.toolGroup)
				sb.WriteString(toolDoneStyle.Render("  " + summary))
				sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("  (ctrl+o to expand)"))
				sb.WriteString("\n")
			}

		case "sub_agent":
			if msg.subAgentBlock != nil {
				sb.WriteString(renderSubAgentBlock(msg.subAgentBlock, msg.expanded))
			}

		case "thinking":
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(msg.content))
			sb.WriteString("\n\n")

		case "system":
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(msg.content))
			sb.WriteString("\n\n")

		case "error":
			sb.WriteString(errorStyle.Render("✗ " + msg.content))
			sb.WriteString("\n\n")
		}
	}

	// Active sub-agent progress (live)
	if m.activeSubAgent != nil && !m.activeSubAgent.done {
		sb.WriteString(renderSubAgentBlock(m.activeSubAgent, false))
	}

	// Active tool blocks
	for _, tb := range m.toolBlocks {
		if tb.toolName == "Agent" && m.activeSubAgent != nil {
			continue // rendered above as sub-agent block
		}
		sb.WriteString(m.renderToolBlock(tb))
		sb.WriteString("\n")
	}

	// Streaming text
	if m.streaming && m.streamBuf != "" {
		sb.WriteString(aiMarkerStyle.Render("● "))
		indented := indentBlock(m.streamBuf, "  ")
		sb.WriteString(streamingTextStyle.Render(strings.TrimLeft(indented, " ")))
		sb.WriteString("\n")
	}

	// Spinner — always last while agent is running
	if m.streaming {
		elapsed := time.Since(m.thinkingStart).Seconds()
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(brandPurple).Render(
			fmt.Sprintf("  %s %s…  (%.0fs)", m.spinner.View(), m.thinkingVerb, elapsed),
		))
		sb.WriteString("\n")
		// Teammate progress tree
		sb.WriteString(m.renderTeammateTree())
	}

	// Show teammate tree even when not streaming (teammates may still be running)
	if !m.streaming {
		tree := m.renderTeammateTree()
		if tree != "" {
			sb.WriteString(tree)
		}
	}

	return sb.String()
}

func (m Model) renderMessagesRange(from, to int) string {
	var sb strings.Builder
	for i := from; i < to && i < len(m.chatMessages); i++ {
		msg := m.chatMessages[i]
		switch msg.role {
		case "user":
			sb.WriteString(promptStyle.Render("> "))
			sb.WriteString(lipgloss.NewStyle().Foreground(brightText).Bold(true).Render(msg.content))
			sb.WriteString("\n\n")
		case "assistant":
			sb.WriteString(aiMarkerStyle.Render("● "))
			rendered := m.renderMarkdown(msg.content)
			indented := indentBlock(rendered, "    ")
			sb.WriteString(strings.TrimLeft(indented, " "))
			sb.WriteString("\n\n")
		case "tool", "tool_visible":
			sb.WriteString("  ")
			if strings.HasPrefix(msg.content, "✗") {
				sb.WriteString(toolErrorStyle.Render(msg.content))
			} else {
				sb.WriteString(toolDoneStyle.Render(msg.content))
			}
			sb.WriteString("\n")
			appendEditDiff(&sb, msg.toolGroup)
		case "tool_collapsed":
			// Scrollback can't be re-expanded later, so always render each
			// tool inline (with name + args) instead of collapsing the group.
			for _, tb := range msg.toolGroup {
				sb.WriteString("  ")
				text := renderToolBlockText(tb)
				if tb.isError {
					sb.WriteString(toolErrorStyle.Render(text))
				} else {
					sb.WriteString(toolDoneStyle.Render(text))
				}
				sb.WriteString("\n")
			}
		case "tool_group":
			summary := renderToolGroupSummary(msg.toolGroup)
			sb.WriteString(toolDoneStyle.Render("  " + summary))
			sb.WriteString("\n")
		case "sub_agent":
			if msg.subAgentBlock != nil {
				sb.WriteString(renderSubAgentBlock(msg.subAgentBlock, false))
			}
		case "thinking":
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(msg.content))
			sb.WriteString("\n\n")
		case "system":
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(msg.content))
			sb.WriteString("\n\n")
		case "error":
			sb.WriteString(errorStyle.Render("✗ " + msg.content))
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

func (m Model) renderToolBlock(tb toolBlockInfo) string {
	title := toolTitle(tb.toolName, tb.args)

	if tb.loading {
		return toolRunningStyle.Render(fmt.Sprintf("● %s …", title))
	}

	if tb.isError {
		return toolErrorStyle.Render(fmt.Sprintf("✗ %s — error (%.1fs)", title, tb.elapsed))
	}

	return toolDoneStyle.Render(fmt.Sprintf("✓ %s (%.1fs)", title, tb.elapsed))
}

var menuActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))

func (m Model) renderSlashMenu() string {
	var sb strings.Builder
	for i, cmd := range m.slashMatches {
		desc := cmd.Description
		if len([]rune(desc)) > 30 {
			desc = string([]rune(desc)[:28]) + "…"
		}
		label := fmt.Sprintf("  /%-16s — %s", cmd.Name, desc)
		if i == m.slashCursor {
			sb.WriteString(menuActiveStyle.Render(label) + "\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render(label) + "\n")
		}
	}
	return sb.String()
}

func (m Model) renderAtMenu() string {
	var sb strings.Builder
	for i, path := range m.atMatches {
		if i == m.atCursor {
			sb.WriteString(menuActiveStyle.Render("  "+path) + "\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).Render("  "+path) + "\n")
		}
	}
	return sb.String()
}

func (m Model) renderPermDialog() string {
	var sb strings.Builder

	// Command header
	sb.WriteString(permBorderStyle.Render(fmt.Sprintf("  %s command", m.permToolName)))
	sb.WriteString("\n\n")

	// Command detail
	desc := m.permDesc
	if desc != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(normalText).PaddingLeft(4).Render(desc))
		sb.WriteString("\n\n")
	}

	// Approval notice
	sb.WriteString(permDimStyle.Render("  This command requires approval"))
	sb.WriteString("\n\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(normalText).PaddingLeft(2).Render("Do you want to proceed?"))
	sb.WriteString("\n")

	// Selectable options
	for i, opt := range permOptions {
		prefix := "   "
		style := lipgloss.NewStyle().Foreground(dimText)
		if i == m.permCursor {
			prefix = " > "
			style = lipgloss.NewStyle().Foreground(cyanText)
		}
		label := fmt.Sprintf("%d. %s", i+1, opt.label)
		sb.WriteString(style.Render(prefix+label) + "\n")
	}

	sb.WriteString("\n")
	return sb.String()
}

func (m Model) renderRewindDialog() string {
	if m.rewindPhase == 1 {
		return m.renderRewindOptionsDialog()
	}
	return m.renderRewindSnapshotList()
}

func (m Model) renderRewindSnapshotList() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(brandPurple).Bold(true).Render("  ⟲ Rewind to checkpoint"))
	sb.WriteString("\n\n")

	maxVisible := 8
	start := 0
	if m.rewindCursor >= maxVisible {
		start = m.rewindCursor - maxVisible + 1
	}
	end := min(start+maxVisible, len(m.rewindSnapshots))

	for i := start; i < end; i++ {
		snap := m.rewindSnapshots[i]
		prefix := "   "
		style := lipgloss.NewStyle().Foreground(dimText)
		if i == m.rewindCursor {
			prefix = " > "
			style = lipgloss.NewStyle().Foreground(cyanText)
		}
		ago := time.Since(snap.Timestamp).Truncate(time.Second)
		label := snap.UserText
		if len(label) > 50 {
			label = label[:50] + "…"
		}
		files := len(snap.Backups)
		line := fmt.Sprintf("%s[%d] %s (%s ago, %d file(s))", prefix, i+1, label, ago, files)
		sb.WriteString(style.Render(line) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render("↑/↓ navigate · enter select · esc cancel"))
	sb.WriteString("\n")
	return sb.String()
}

func (m Model) renderRewindOptionsDialog() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(brandPurple).Bold(true).Render("  ⟲ Rewind to checkpoint"))
	sb.WriteString("\n\n")

	snap := m.rewindSnapshots[m.rewindCursor]
	ago := time.Since(snap.Timestamp).Truncate(time.Second)
	label := snap.UserText
	if len(label) > 50 {
		label = label[:50] + "…"
	}
	selected := fmt.Sprintf("  Selected: [%d] %s (%s ago, %d file(s))", m.rewindCursor+1, label, ago, len(snap.Backups))
	sb.WriteString(lipgloss.NewStyle().Foreground(normalText).Render(selected))
	sb.WriteString("\n\n")

	for i, opt := range rewindOptions {
		prefix := "   "
		style := lipgloss.NewStyle().Foreground(dimText)
		if i == m.rewindOptionCursor {
			prefix = " > "
			style = lipgloss.NewStyle().Foreground(cyanText)
		}
		sb.WriteString(style.Render(prefix+opt) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render("↑/↓ navigate · enter select · esc back"))
	sb.WriteString("\n")
	return sb.String()
}

func (m Model) renderMarkdown(content string) string {
	// Don't use WithAutoStyle — it queries the terminal background via OSC 11
	// every time, and the response leaks into stdin and pollutes the input.
	// Force TrueColor explicitly: without a profile, glamour delegates to
	// termenv auto-detection, which fails under bubbletea's stdin takeover
	// and falls back to the no-color "notty" style — markdown then prints raw.
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(m.width-6),
	)
	if err != nil {
		return content
	}
	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(rendered)
}

// ── Helpers ──

func toolTitle(toolName string, args map[string]any) string {
	switch toolName {
	case "ReadFile":
		p, _ := args["file_path"].(string)
		if p != "" {
			return "Read " + filepath.Base(p)
		}
		return "Read"
	case "WriteFile":
		p, _ := args["file_path"].(string)
		content, _ := args["content"].(string)
		lines := strings.Count(content, "\n") + 1
		if p != "" {
			return fmt.Sprintf("Write %s (%d lines)", filepath.Base(p), lines)
		}
		return "Write"
	case "EditFile":
		p, _ := args["file_path"].(string)
		if p != "" {
			return "Edit " + filepath.Base(p)
		}
		return "Edit"
	case "Bash":
		cmd, _ := args["command"].(string)
		if len(cmd) > 50 {
			cmd = cmd[:50] + "…"
		}
		if cmd != "" {
			return fmt.Sprintf("Bash: %s", cmd)
		}
		return "Bash"
	case "Glob":
		pattern, _ := args["pattern"].(string)
		return fmt.Sprintf("Glob: %s", pattern)
	case "Grep":
		pattern, _ := args["pattern"].(string)
		return fmt.Sprintf("Grep: %s", pattern)
	}
	return toolName
}

func indentBlock(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) pastTense(verb string) string {
	if before, ok := strings.CutSuffix(verb, "ing"); ok {
		stem := before
		if strings.HasSuffix(stem, "at") || strings.HasSuffix(stem, "ut") ||
			strings.HasSuffix(stem, "it") || strings.HasSuffix(stem, "et") {
			return stem + "ed"
		}
		if strings.HasSuffix(stem, "e") {
			return stem + "d"
		}
		return stem + "ed"
	}
	return verb
}

func (m Model) getModelName() string {
	if m.selectedProvider != nil {
		return m.selectedProvider.Model
	}
	if len(m.providers) > 0 {
		return m.providers[0].Model
	}
	return "unknown"
}

func (m Model) getWorkDir() string {
	wd, _ := os.Getwd()
	return wd
}

func nextPermissionMode(current permissions.PermissionMode) permissions.PermissionMode {
	switch current {
	case permissions.ModeDefault:
		return permissions.ModeAcceptEdits
	case permissions.ModeAcceptEdits:
		return permissions.ModePlan
	case permissions.ModePlan:
		return permissions.ModeBypass
	case permissions.ModeBypass:
		return permissions.ModeDefault
	default:
		return permissions.ModeDefault
	}
}

func (m Model) handleResume(args string) (tea.Model, tea.Cmd) {
	wd, _ := os.Getwd()
	sessions := session.ListSessions(wd)

	if args != "" {
		return m.doResumeSession(wd, args, sessions)
	}

	if len(sessions) == 0 {
		m.chatMessages = append(m.chatMessages, chatMessage{
			role: "system", content: "No previous sessions found.",
		})
		m.updateViewport()
		return m, nil
	}

	m.resumeSessions = sessions
	m.resumeFiltered = sessions
	m.resumeCursor = 0
	m.resumeSearch = ""
	m.resumeScrollTop = 0
	m.state = stateResume
	return m, nil
}

func (m Model) doResumeSession(wd, targetID string, sessions []session.SessionInfo) (tea.Model, tea.Cmd) {
	var idx int
	if n, _ := fmt.Sscanf(targetID, "%d", &idx); n == 1 && idx >= 1 && idx <= len(sessions) {
		targetID = sessions[idx-1].ID
	}

	msgs := session.LoadSession(wd, strings.TrimSpace(targetID))
	if len(msgs) == 0 {
		m.chatMessages = append(m.chatMessages, chatMessage{
			role: "error", content: fmt.Sprintf("Session '%s' not found or empty.", targetID),
		})
		m.updateViewport()
		return m, nil
	}

	m.chatMessages = nil
	m.committedUpTo = 0
	m.conversation = conversation.NewManager()
	m.sessionID = strings.TrimSpace(targetID)
	// Keep the Agent's session log pointer in sync with the resumed session so a
	// later compaction writes its boundary into this same file (chained resume).
	if m.ag != nil {
		m.ag.SetSessionID(m.sessionID)
	}

	// Compaction-aware rebuild: if the session contains a compact_boundary, the
	// live conversation is the compacted state — [summary] + kept tail + any
	// plain messages appended after the boundary — and the original
	// pre-compaction prefix is NOT replayed (it stays in the file for audit).
	// Without a boundary (old sessions) we replay everything verbatim.
	boundary, after, compacted := session.FindLastCompactBoundary(msgs)
	var replay []session.Message
	if compacted {
		resumeSummary := "This session continues from a previous conversation that was compacted due to insufficient context space. Below is a summary of the earlier conversation:\n\n" + boundary.Summary
		if len(boundary.Keep) > 0 {
			resumeSummary += "\n\nRecent messages have been preserved verbatim."
		}
		replay = append(replay, session.Message{Role: "user", Content: resumeSummary})
		for _, k := range boundary.Keep {
			replay = append(replay, session.Message{
				Role:        k.Role,
				Content:     k.Content,
				ToolUses:    k.ToolUses,
				ToolResults: k.ToolResults,
			})
		}
		replay = append(replay, after...)
	} else {
		replay = msgs
	}

	for _, msg := range replay {
		// Messages containing only tool results have no text; skip rendering
		// but still append to conversation history to preserve the call chain
		if msg.Content != "" {
			m.chatMessages = append(m.chatMessages, chatMessage{role: msg.Role, content: msg.Content})
		}
		m.conversation.AppendMessages([]conversation.Message{msg.ToConversation()})
	}

	restored := fmt.Sprintf("Session %s restored (%d messages).", strings.TrimSpace(targetID), len(replay))
	if compacted {
		restored = fmt.Sprintf("Session %s restored from compacted state (summary + %d kept + %d newer messages).",
			strings.TrimSpace(targetID), len(boundary.Keep), len(after))
	}
	m.chatMessages = append(m.chatMessages, chatMessage{
		role:    "system",
		content: restored,
	})
	commitText := m.renderMessagesRange(0, len(m.chatMessages))
	m.committedUpTo = len(m.chatMessages)
	m.updateViewport()
	if commitText != "" {
		return m, tea.Println(commitText)
	}
	return m, nil
}

func (m Model) handleRewind() (tea.Model, tea.Cmd) {
	if m.fileHistory == nil {
		m.chatMessages = append(m.chatMessages, chatMessage{
			role: "system", content: "No file history available (fileHistory is nil).",
		})
		m.updateViewport()
		return m, nil
	}
	if !m.fileHistory.HasSnapshots() {
		m.chatMessages = append(m.chatMessages, chatMessage{
			role: "system", content: "No checkpoints to rewind to (0 snapshots).",
		})
		m.updateViewport()
		return m, nil
	}
	m.rewindSnapshots = m.fileHistory.GetSnapshots()
	m.rewindCursor = len(m.rewindSnapshots) - 1
	m.rewindPhase = 0
	m.rewindOptionCursor = 0
	m.rewindDialog = true
	m.updateViewport()
	return m, nil
}

var rewindOptions = []string{
	"Restore code and conversation",
	"Restore conversation only",
	"Restore code only",
	"Never mind",
}

func (m Model) handleRewindKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.rewindPhase == 0 {
		return m.handleRewindPhase0(msg)
	}
	return m.handleRewindPhase1(msg)
}

func (m Model) handleRewindPhase0(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "escape":
		m.rewindDialog = false
		m.textarea.Focus()
		m.updateViewport()
		return m, nil
	case "up":
		if m.rewindCursor > 0 {
			m.rewindCursor--
			m.updateViewport()
		}
		return m, nil
	case "down":
		if m.rewindCursor < len(m.rewindSnapshots)-1 {
			m.rewindCursor++
			m.updateViewport()
		}
		return m, nil
	case "enter":
		if m.rewindCursor < len(m.rewindSnapshots) {
			m.rewindPhase = 1
			m.rewindOptionCursor = 0
			m.updateViewport()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleRewindPhase1(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "escape":
		m.rewindPhase = 0
		m.updateViewport()
		return m, nil
	case "up":
		if m.rewindOptionCursor > 0 {
			m.rewindOptionCursor--
			m.updateViewport()
		}
		return m, nil
	case "down":
		if m.rewindOptionCursor < len(rewindOptions)-1 {
			m.rewindOptionCursor++
			m.updateViewport()
		}
		return m, nil
	case "enter":
		return m.executeRewindOption()
	}
	return m, nil
}

func (m Model) executeRewindOption() (tea.Model, tea.Cmd) {
	snap := m.rewindSnapshots[m.rewindCursor]
	var summary string

	switch m.rewindOptionCursor {
	case 0: // Restore code and conversation
		changed, err := m.fileHistory.Rewind(m.rewindCursor)
		if err != nil {
			m.chatMessages = append(m.chatMessages, chatMessage{role: "error", content: fmt.Sprintf("Rewind failed: %s", err)})
			m.rewindDialog = false
			m.textarea.Focus()
			m.updateViewport()
			return m, nil
		}
		m.conversation.TruncateTo(snap.MessageIndex)
		summary = fmt.Sprintf("⟲ Rewound to checkpoint %d. Restored %d file(s) and conversation.", m.rewindCursor+1, len(changed))
		for _, f := range changed {
			summary += "\n  • " + f
		}
		m.chatMessages = m.chatMessages[:0]
		m.committedUpTo = 0

	case 1: // Restore conversation only
		m.conversation.TruncateTo(snap.MessageIndex)
		summary = fmt.Sprintf("⟲ Rewound conversation to checkpoint %d. Files unchanged.", m.rewindCursor+1)
		m.chatMessages = m.chatMessages[:0]
		m.committedUpTo = 0

	case 2: // Restore code only
		changed, err := m.fileHistory.Rewind(m.rewindCursor)
		if err != nil {
			m.chatMessages = append(m.chatMessages, chatMessage{role: "error", content: fmt.Sprintf("Rewind failed: %s", err)})
			m.rewindDialog = false
			m.textarea.Focus()
			m.updateViewport()
			return m, nil
		}
		summary = fmt.Sprintf("⟲ Restored %d file(s) to checkpoint %d. Conversation unchanged.", len(changed), m.rewindCursor+1)
		for _, f := range changed {
			summary += "\n  • " + f
		}

	case 3: // Never mind
		m.rewindDialog = false
		m.rewindPhase = 0
		m.textarea.Focus()
		m.updateViewport()
		return m, nil
	}

	m.chatMessages = append(m.chatMessages, chatMessage{role: "system", content: summary})
	m.rewindDialog = false
	m.rewindPhase = 0
	m.textarea.Focus()
	m.updateViewport()

	if m.rewindOptionCursor <= 1 {
		return m, tea.Batch(
			func() tea.Msg { return tea.ClearScreen() },
			tea.Println(m.renderBanner()+"\n"+lipgloss.NewStyle().Foreground(dimText).Render(summary)),
		)
	}
	commitText := m.renderMessagesRange(m.committedUpTo-1, len(m.chatMessages))
	m.committedUpTo = len(m.chatMessages)
	if commitText != "" {
		return m, tea.Println(commitText)
	}
	return m, nil
}

func (m Model) handleResumeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "escape":
		m.state = stateChat
		m.textarea.Focus()
		return m, nil
	case "enter":
		if m.resumeCursor < len(m.resumeFiltered) {
			selected := m.resumeFiltered[m.resumeCursor]
			m.state = stateChat
			m.textarea.Focus()
			wd, _ := os.Getwd()
			return m.doResumeSession(wd, selected.ID, m.resumeSessions)
		}
		return m, nil
	case "up":
		if m.resumeCursor > 0 {
			m.resumeCursor--
			if m.resumeCursor < m.resumeScrollTop {
				m.resumeScrollTop = m.resumeCursor
			}
		}
		return m, nil
	case "down":
		if m.resumeCursor < len(m.resumeFiltered)-1 {
			m.resumeCursor++
			maxVisible := m.resumeVisibleCount()
			if m.resumeCursor >= m.resumeScrollTop+maxVisible {
				m.resumeScrollTop = m.resumeCursor - maxVisible + 1
			}
		}
		return m, nil
	case "backspace":
		if len(m.resumeSearch) > 0 {
			m.resumeSearch = m.resumeSearch[:len(m.resumeSearch)-1]
			m.resumeFilterSessions()
		}
		return m, nil
	default:
		if len(msg.String()) == 1 && msg.String() >= " " {
			m.resumeSearch += msg.String()
			m.resumeFilterSessions()
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) resumeFilterSessions() {
	m.resumeFiltered = nil
	for _, s := range m.resumeSessions {
		if session.MatchesSearch(s, m.resumeSearch) {
			m.resumeFiltered = append(m.resumeFiltered, s)
		}
	}
	m.resumeCursor = 0
	m.resumeScrollTop = 0
}

func (m Model) resumeVisibleCount() int {
	// header(1) + search box(3) + project(1) + blank(1) + footer(2) = 8
	available := m.height - 8
	perItem := 2 // title line + metadata line
	if available < perItem {
		return 1
	}
	return available / perItem
}

func (m Model) renderResumeView() string {
	var sb strings.Builder

	total := len(m.resumeFiltered)
	current := 0
	if total > 0 {
		current = m.resumeCursor + 1
	}

	// Header
	sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(
		fmt.Sprintf("Resume session (%d of %d)", current, total),
	))
	sb.WriteString("\n")

	// Search box
	searchText := m.resumeSearch
	if searchText == "" {
		searchText = lipgloss.NewStyle().Foreground(dimText).Render("⌕ Search…")
	} else {
		searchText = "⌕ " + searchText
	}
	boxWidth := max(m.width-6, 20)
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(boxWidth).
		PaddingLeft(1)
	sb.WriteString(lipgloss.NewStyle().PaddingLeft(2).Render(border.Render(searchText)))
	sb.WriteString("\n")

	// Project name
	wd, _ := os.Getwd()
	projectName := filepath.Base(wd)
	sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(4).Render(projectName))
	sb.WriteString("\n\n")

	// Session list
	maxVisible := min(m.resumeVisibleCount(), total)

	for i := m.resumeScrollTop; i < m.resumeScrollTop+maxVisible && i < total; i++ {
		s := m.resumeFiltered[i]

		// Title line
		title := s.FirstMessage
		if title == "" {
			title = "(empty session)"
		}
		maxTitleLen := m.width - 8
		if maxTitleLen > 0 && len(title) > maxTitleLen {
			title = title[:maxTitleLen] + "…"
		}

		prefix := "  "
		if i == m.resumeCursor {
			prefix = "> "
			title = lipgloss.NewStyle().Foreground(cyanText).Bold(true).Render(title)
		} else {
			title = lipgloss.NewStyle().Foreground(normalText).Render(title)
		}
		sb.WriteString(lipgloss.NewStyle().PaddingLeft(2).Render(prefix + title))
		sb.WriteString("\n")

		// Metadata line
		var meta []string
		meta = append(meta, session.FormatRelativeTime(s.ModTime))
		if s.GitBranch != "" {
			meta = append(meta, s.GitBranch)
		}
		meta = append(meta, session.FormatFileSize(s.FileSize))
		metaStr := strings.Join(meta, " · ")
		sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(6).Render(metaStr))
		sb.WriteString("\n")

		if i < m.resumeScrollTop+maxVisible-1 && i < total-1 {
			sb.WriteString("\n")
		}
	}

	// Show scroll indicator
	if total > maxVisible {
		if m.resumeScrollTop+maxVisible < total {
			sb.WriteString("\n")
			sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(2).Render(
				fmt.Sprintf("  ↓ %d more session(s)", total-m.resumeScrollTop-maxVisible),
			))
		}
	}

	// Pad to bottom
	rendered := strings.Count(sb.String(), "\n") + 1
	footerHeight := 2
	pad := m.height - rendered - footerHeight
	if pad > 0 {
		sb.WriteString(strings.Repeat("\n", pad))
	}

	// Footer
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(dimText).PaddingLeft(4).Render(
		"Type to search · Enter to select · Esc to cancel",
	))

	return sb.String()
}

func (m *Model) historyUp() {
	if len(m.historyEntries) == 0 {
		return
	}
	if m.historyIndex == 0 {
		m.historyDraft = m.textarea.Value()
	}
	if m.historyIndex < len(m.historyEntries) {
		m.historyIndex++
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		m.textarea.SetValue(m.historyEntries[len(m.historyEntries)-m.historyIndex])
	}
}

func (m *Model) historyDown() {
	if m.historyIndex <= 0 {
		return
	}
	m.historyIndex--
	m.textarea.Reset()
	m.textarea.SetHeight(1)
	if m.historyIndex == 0 {
		m.textarea.SetValue(m.historyDraft)
	} else {
		m.textarea.SetValue(m.historyEntries[len(m.historyEntries)-m.historyIndex])
	}
}

func permissionModeInfo(mode permissions.PermissionMode) (string, string) {
	switch mode {
	case permissions.ModeDefault:
		return "Default", "Writes and commands require approval."
	case permissions.ModeAcceptEdits:
		return "Accept Edits", "File edits auto-approved; commands still require approval."
	case permissions.ModePlan:
		return "Plan", "Read-only mode. No writes or commands executed."
	case permissions.ModeBypass:
		return "YOLO", "All tools auto-approved. Use with caution."
	default:
		return string(mode), ""
	}
}
