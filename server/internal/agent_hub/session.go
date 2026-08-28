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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty.go/swifty_http"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/commands"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/compact"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/file_history"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/hooks"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/mcp"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory/extractor"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/plan_file"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/prompt"
	swiftx_session "github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/skills"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/subagent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/teams"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/todo"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// promptQueueSize bounds how many turns may wait while one is running. Past
// that the user is told to slow down rather than silently piling up work.
const promptQueueSize = 8

// promptJob is one queued turn, already persisted by the chat pipeline.
type promptJob struct {
	chatSessionID string
	messageID     string
	content       string
}

// Session is one user's private agent: its own conversation, tool registry,
// slash-command state and sandboxed workspace. Everything after start() that
// touches the agent runs on the single worker goroutine, so a user's turns are
// strictly serialized and the registry is never mutated concurrently.
type Session struct {
	userID   string
	workDir  string
	sink     ChatSink
	provider config.ProviderConfig
	appCfg   *config.AppConfig
	// mgr is the owner, consulted for anything shared between users — today
	// that is the MCP connection pool.
	mgr *Manager

	connMu sync.Mutex
	conns  map[*swifty_http.WSConn]struct{}

	queue    chan promptJob
	done     chan struct{}
	stopOnce sync.Once

	// Worker-goroutine state.
	ag              *agent.Agent
	conv            *conversation.Manager
	registry        *tools.Registry
	defaultTools    tools.DefaultTools
	client          llm.Client
	sessionID       string
	fileHistory     *file_history.History
	askUserCh       chan tools.AskUserRequest
	cmdRegistry     *commands.Registry
	skillCatalog    *skills.Catalog
	taskMgr         *subagent.TaskManager
	todoList        *todo.TaskList
	memoryMgr       *memory.Manager
	memoryExtractor *extractor.Extractor
	teamMgr         *teams.TeamManager
	mcpToolCount    int
	instructions    string
	memoryContent   string
	skillSection    string
	mcpInstructions string
	agentCh         <-chan agent.AgentEvent
	// chatSessionID is the chat conversation the turn being served belongs to,
	// so replies are filed under the same session as the prompt.
	chatSessionID string

	stateMu    sync.Mutex
	streaming  bool
	cancelRun  context.CancelFunc
	cancelled  bool
	ready      bool
	lastActive time.Time
	// anchorID is the chat message new ephemeral items belong after, so a
	// client that connects mid-run still places tool cards correctly.
	anchorID  string
	lastUsage [2]int

	pendingMu sync.Mutex
	// Requests are kept alongside their reply channel so a reconnecting client
	// can be shown the prompt it still owes an answer to.
	pendingPerms map[string]chan<- agent.PermissionResponse
	pendingAsks  map[string]chan tools.QuestionResponse
	pendingEvent map[string]Event
}

func newSession(mgr *Manager, userID, workDir string) (*Session, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	s := &Session{
		userID:  userID,
		workDir: workDir,
		mgr:     mgr,
		sink:    mgr.sink,
		// A copy: each session resolves and caches its own context window.
		provider:     mgr.provider,
		appCfg:       mgr.cfg,
		conns:        make(map[*swifty_http.WSConn]struct{}),
		queue:        make(chan promptJob, promptQueueSize),
		done:         make(chan struct{}),
		lastActive:   time.Now(),
		pendingPerms: make(map[string]chan<- agent.PermissionResponse),
		pendingAsks:  make(map[string]chan tools.QuestionResponse),
		pendingEvent: make(map[string]Event),
	}
	if err := s.initAgent(); err != nil {
		return nil, err
	}
	go s.worker()
	return s, nil
}

// initAgent mirrors the swiftx TUI's single-provider startup, scoped to this
// user's workspace. Everything derived from a working directory — skills,
// instructions, memory, sessions, todos, the permission sandbox — therefore
// stays inside that user's directory.
func (s *Session) initAgent() error {
	p := &s.provider
	wd := s.workDir

	s.askUserCh = make(chan tools.AskUserRequest, 1)
	s.defaultTools = tools.CreateDefaultToolsWithWorkDir(wd)
	s.registry = s.defaultTools.Registry
	s.registry.Register(&tools.AskUserQuestionTool{RequestCh: s.askUserCh})

	s.cmdRegistry = commands.CreateDefaultRegistry()
	s.skillCatalog = skills.LoadCatalog(wd)
	s.instructions = buildInstructions(wd)
	s.memoryContent = memory.LoadAutoMemoryPrompt(wd)
	s.skillSection = buildSkillSection(s.skillCatalog, wd)

	env := prompt.DetectEnvironment(wd)
	env.Model = p.Model
	client, err := llm.NewClient(p, prompt.BuildSystemPrompt(env))
	if err != nil {
		return err
	}
	s.client = client
	s.conv = conversation.NewManager()
	s.sessionID = swiftx_session.NewID()
	s.fileHistory = file_history.New(wd, s.sessionID)
	s.defaultTools.EditFile.FileHistory = s.fileHistory
	s.defaultTools.WriteFile.FileHistory = s.fileHistory

	s.registerTools(client, p, wd)

	ag := agent.New(client, s.registry, p.Protocol)
	ag.Instructions = s.instructions
	ag.MemoryContent = s.memoryContent
	ag.SkillSection = s.skillSection
	ag.FileHistory = s.fileHistory
	ag.SetSessionID(s.sessionID)

	// The sandbox is the whole isolation story between chat users: only this
	// user's workspace is reachable, and the shared ~/.swiftx memory is
	// deliberately left out so one user cannot write what another one reads.
	ag.Checker = permissions.NewChecker(
		permissions.NewPathSandbox(wd, memory.GetAutoMemPath(wd)),
		permissions.NewRuleEngine(wd),
		resolveMode(s.appCfg.PermissionMode),
	)

	if len(s.appCfg.Hooks) > 0 {
		eng := hooks.NewEngine()
		eng.LoadHooks(s.appCfg.Hooks)
		eng.AgentRunner = newAgentHookRunner(client)
		ag.Hooks = eng
	}

	coordinator := s.appCfg.EnableCoordinatorMode
	ag.NotificationFn = func() []string {
		var messages []string
		if s.taskMgr != nil {
			for _, n := range s.taskMgr.DrainNotifications() {
				messages = append(messages, fmt.Sprintf(
					"<task-notification>\n<task_id>%s</task_id>\n<status>%s</status>\n<summary>Agent \"%s\" %s</summary>\n<result>%s</result>\n</task-notification>",
					n.TaskID, n.Status, n.Name, n.Status, n.Output))
			}
		}
		return append(messages, teams.DrainLeadMailbox(s.teamMgr)...)
	}
	ag.ToolNameFilter = teams.CoordinatorToolFilter(coordinator)
	ag.CoordinatorActiveFn = teams.CoordinatorActiveFn(coordinator)

	s.ag = ag
	if at, ok := s.registry.Get("Agent").(*subagent.AgentTool); ok {
		at.ParentChecker = ag.Checker
	}
	s.registry.Register(&skills.LoadSkillTool{Catalog: s.skillCatalog, Host: s})
	s.memoryExtractor = installMemExtractor(ag, wd, p.Protocol, client, s.registry, s.conv)
	return nil
}

func (s *Session) registerTools(client llm.Client, p *config.ProviderConfig, wd string) {
	s.taskMgr = subagent.NewTaskManager()
	s.todoList = todo.NewTaskList(todo.NewStore(wd, s.sessionID))
	s.memoryMgr = memory.NewManager(wd)
	loader := subagent.NewAgentLoader(wd)
	loader.LoadAll()
	s.teamMgr = teams.NewTeamManager()

	s.registry.Register(&tools.ExitPlanModeTool{
		IsPlanMode: func() bool {
			return s.ag != nil && s.ag.Checker != nil && s.ag.Checker.Mode == permissions.ModePlan
		},
		PlanExists: func() bool { return false },
	})
	s.registry.Register(&todo.TaskCreateTool{List: s.todoList})
	s.registry.Register(&todo.TaskGetTool{List: s.todoList})
	s.registry.Register(&todo.TaskListTool{List: s.todoList})
	s.registry.Register(&todo.TaskUpdateTool{List: s.todoList})
	s.registry.Register(&tools.ToolSearchTool{Registry: s.registry, Protocol: p.Protocol})
	s.registry.Register(&tools.McpCallTool{Registry: s.registry})
	s.registry.Register(&teams.TeamCreateTool{TeamMgr: s.teamMgr})
	s.registry.Register(&teams.TeamDeleteTool{TeamMgr: s.teamMgr})
	s.registry.Register(&teams.SendMessageTool{TeamMgr: s.teamMgr, SenderName: "lead"})
	s.registry.Register(&teams.TaskStopTool{TeamMgr: s.teamMgr})
	s.registry.Register(&tools.SyntheticOutputTool{})
	s.registry.Register(&subagent.AgentTool{
		Client:        client,
		ModelResolver: llm.NewModelResolver(*p),
		Registry:      s.registry,
		Protocol:      p.Protocol,
		TaskMgr:       s.taskMgr,
		ProgressCh:    make(chan subagent.SubAgentProgress, 32),
		Loader:        loader,
		Conversation:  s.conv,
		TeamMgr:       s.teamMgr,
		ForkDisabled:  !s.appCfg.ForkEnabled(),
	})
}

// restoreLatestSession reloads the user's most recent transcript so context
// survives an idle eviction or a server restart. The chat history the user sees
// lives in MongoDB; this only rebuilds what the model remembers.
func (s *Session) restoreLatestSession() {
	sessions := swiftx_session.ListSessions(s.workDir)
	if len(sessions) == 0 {
		return
	}
	s.loadSessionContext(sessions[0].ID)
}

// loadSessionContext replaces the model's context with a stored transcript and
// adopts its id, so further turns append to the same file. It reports how many
// messages were replayed and whether they came from a compacted checkpoint.
func (s *Session) loadSessionContext(id string) (int, bool) {
	msgs := swiftx_session.LoadSession(s.workDir, id)
	if len(msgs) == 0 {
		return 0, false
	}
	s.sessionID = id
	s.ag.SetSessionID(id)
	s.fileHistory = file_history.New(s.workDir, id)
	s.ag.FileHistory = s.fileHistory
	s.defaultTools.EditFile.FileHistory = s.fileHistory
	s.defaultTools.WriteFile.FileHistory = s.fileHistory

	boundary, after, compacted := swiftx_session.FindLastCompactBoundary(msgs)
	replay := msgs
	if compacted {
		summary := "This session continues from a previous conversation that was compacted due to context size limits. Below is a summary of the earlier discussion:\n\n" + boundary.Summary
		if len(boundary.Keep) > 0 {
			summary += "\n\nRecent messages have been preserved as-is."
		}
		replay = []swiftx_session.Message{{Role: "user", Content: summary}}
		for _, k := range boundary.Keep {
			replay = append(replay, swiftx_session.Message{
				Role: k.Role, Content: k.Content, ToolUses: k.ToolUses, ToolResults: k.ToolResults,
			})
		}
		replay = append(replay, after...)
	}

	s.conv.Reset()
	for _, msg := range replay {
		s.conv.AppendMessages([]conversation.Message{msg.ToConversation()})
	}
	return len(replay), compacted
}

// worker owns every mutation of the agent. Provider resolution, transcript
// restore and MCP startup happen here rather than in newSession so their I/O —
// and a slow or hanging MCP server — delays only its own user instead of the
// chat event loop that dispatched the message.
func (s *Session) worker() {
	llm.ResolveContextWindow(context.Background(), &s.provider)
	s.ag.ContextWindow = s.provider.GetContextWindow()
	s.ag.MaxOutputTokens = s.provider.GetMaxOutputTokens()
	s.restoreLatestSession()
	s.initMCP()

	// Startup can take tens of seconds when MCP servers are configured, and a
	// prompt sent in the meantime just waits in the queue. Clients are told the
	// moment that is over so they can stop showing the agent as warming up.
	s.stateMu.Lock()
	s.ready = true
	s.stateMu.Unlock()
	s.emit(Event{Type: "ready"})

	for {
		select {
		case <-s.done:
			return
		case job := <-s.queue:
			s.handlePrompt(job)
		}
	}
}

// initMCP gives this agent its own wrappers over the process-wide MCP
// connections. The connections are shared because each one costs a child
// process or a socket; the wrappers are not, because ApplyMode writes a defer
// flag on them per registry.
func (s *Session) initMCP() {
	mgr, instructions := s.mgr.sharedMCP()
	if mgr == nil {
		return
	}
	toolSet := mgr.NewToolSet()
	if len(toolSet) == 0 {
		// Every server failed. This agent then behaves exactly like one on a
		// host with no MCP configured at all.
		return
	}
	for _, t := range toolSet {
		s.registry.Register(t)
	}
	s.mcpToolCount = len(toolSet)
	mcp.DecideAndApply(s.registry, s.provider.BaseURL, s.provider.GetContextWindow())
	s.mcpInstructions = instructions
}

// enqueue hands a persisted chat message to the worker. A full queue means the
// user is far ahead of the agent, which is worth saying out loud.
func (s *Session) enqueue(job promptJob) {
	select {
	case s.queue <- job:
	default:
		s.emit(Event{Type: "system", Data: map[string]string{
			"message": "Swiftx is still working through earlier messages — please wait for it to catch up.",
		}})
	}
}

func (s *Session) handlePrompt(job promptJob) {
	content := strings.TrimSpace(job.content)
	if content == "" {
		return
	}
	s.chatSessionID = job.chatSessionID
	s.setAnchor(job.messageID)
	s.emit(Event{Type: "run_start", Data: map[string]string{"userMessageId": job.messageID}})

	if strings.HasPrefix(content, "/") {
		s.handleSlashCommand(content)
		return
	}
	s.runTurn(content, content)
}

// runTurn drives one agent loop. saveText is what the transcript records (for a
// slash command that is the command itself); promptText is what the model sees.
func (s *Session) runTurn(saveText, promptText string) {
	swiftx_session.SaveMessage(s.workDir, s.sessionID, swiftx_session.Message{
		Role: "user", Content: saveText, Ts: time.Now().Unix(),
	})
	s.conv.AddUserMessage(promptText)
	if s.mcpInstructions != "" {
		s.conv.AddSystemReminder(s.mcpInstructions)
		s.mcpInstructions = ""
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.beginRun(cancel)
	defer s.endRun()

	s.agentCh = s.ag.Run(ctx, s.conv)
	askDone := make(chan struct{})
	go s.listenForAskUser(askDone)
	s.consumeAgentEvents()
	close(askDone)
	s.agentCh = nil
}

func (s *Session) consumeAgentEvents() {
	streamBuf := ""
	startTime := time.Now()
	completed := false

	for ev := range s.agentCh {
		switch e := ev.(type) {
		case agent.StreamText:
			streamBuf += e.Text
			s.emit(Event{Type: "stream_text", Data: map[string]string{"text": e.Text}})

		case agent.ThinkingText:
			s.emit(Event{Type: "thinking_text", Data: map[string]string{"text": e.Text}})

		case agent.ToolUseEvent:
			// The text that led up to this call is finished, so it is committed
			// before the card is announced: the client anchors the card after
			// the message it follows, and that message has to exist first.
			streamBuf = s.flushText(streamBuf)
			s.emit(Event{Type: "tool_use", Data: map[string]any{
				"toolId": e.ToolID, "toolName": e.ToolName, "args": e.Args,
			}})

		case agent.ToolResultEvent:
			s.emit(Event{Type: "tool_result", Data: map[string]any{
				"toolId": e.ToolID, "toolName": e.ToolName, "output": e.Output,
				"isError": e.IsError, "elapsed": e.Elapsed.Seconds(),
			}})

		case agent.PermissionRequestEvent:
			s.requestPermission(e)

		case agent.AskUserQuestionEvent:
			s.requestAnswers(e.Questions, func(resp tools.QuestionResponse) {
				e.ResponseCh <- resp.Answers
			})

		case agent.TurnComplete:
			streamBuf = s.flushText(streamBuf)
			s.emit(Event{Type: "turn_complete", Data: map[string]int{"turn": e.Turn}})

		case agent.LoopComplete:
			completed = true
			streamBuf = s.flushText(streamBuf)
			s.emit(Event{Type: "loop_complete", Data: map[string]any{
				"totalTurns": e.TotalTurns, "elapsed": time.Since(startTime).Seconds(),
			}})

		case agent.UsageEvent:
			s.recordUsage(e.InputTokens, e.OutputTokens)
			s.emit(Event{Type: "usage", Data: map[string]int{
				"inputTokens": e.InputTokens, "outputTokens": e.OutputTokens,
			}})

		case agent.ErrorEvent:
			// Cancelling a run surfaces as an error from the agent's point of
			// view. Stopping on purpose is not a failure, so it is not shown as
			// one; every other error still is.
			if s.cancelWasRequested() {
				continue
			}
			s.emit(Event{Type: "error", Data: map[string]string{"message": e.Message}})

		case agent.CompactEvent:
			s.emit(Event{Type: "compact", Data: map[string]string{"message": e.Message}})

		case agent.RetryEvent:
			s.emit(Event{Type: "retry", Data: map[string]any{
				"reason": e.Reason, "waitMs": e.Wait.Milliseconds(),
			}})
		}
	}

	if completed {
		return
	}
	// A cancelled run closes its channel without ever reporting completion, so
	// the turn is closed out here: whatever was written is kept, and clients
	// are told the run is over or their composer stays stuck on Stop.
	s.flushText(streamBuf)
	if s.cancelWasRequested() {
		s.emit(Event{Type: "system", Data: map[string]string{"message": "Stopped."}})
	}
	s.emit(Event{Type: "command_done"})
}

// flushText commits one finished text block to the chat transcript and tells
// clients which message replaced the live stream, so the streaming bubble can
// hand over without flicker. Returns the drained buffer.
func (s *Session) flushText(buf string) string {
	if strings.TrimSpace(buf) == "" {
		return ""
	}
	messageID := s.sink.SaveAssistantText(s.userID, s.chatSessionID, buf)
	if messageID != "" {
		s.setAnchor(messageID)
	}
	s.emit(Event{Type: "stream_end", Data: map[string]string{"text": buf, "messageId": messageID}})
	return ""
}

func (s *Session) requestPermission(e agent.PermissionRequestEvent) {
	id := fmt.Sprintf("perm_%d", time.Now().UnixNano())
	ev := Event{Type: "permission_request", Data: map[string]string{
		"id": id, "toolName": e.ToolName, "description": e.Desc,
	}}
	s.pendingMu.Lock()
	s.pendingPerms[id] = e.ResponseCh
	s.pendingEvent[id] = ev
	s.pendingMu.Unlock()
	s.emit(ev)
}

func (s *Session) requestAnswers(questions any, deliver func(tools.QuestionResponse)) {
	id := fmt.Sprintf("ask_%d", time.Now().UnixNano())
	respCh := make(chan tools.QuestionResponse, 1)
	ev := Event{Type: "ask_user", Data: map[string]any{"id": id, "questions": questions}}
	s.pendingMu.Lock()
	s.pendingAsks[id] = respCh
	s.pendingEvent[id] = ev
	s.pendingMu.Unlock()
	s.emit(ev)
	go deliver(<-respCh)
}

// listenForAskUser serves AskUserQuestion calls raised by the tool directly
// rather than through the agent event stream.
func (s *Session) listenForAskUser(done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case req, ok := <-s.askUserCh:
			if !ok {
				return
			}
			s.requestAnswers(req.Questions, func(resp tools.QuestionResponse) {
				req.ResponseCh <- resp
			})
		}
	}
}

func (s *Session) resolvePermission(reply permissionReply) {
	s.pendingMu.Lock()
	ch, ok := s.pendingPerms[reply.ID]
	delete(s.pendingPerms, reply.ID)
	delete(s.pendingEvent, reply.ID)
	s.pendingMu.Unlock()
	if !ok {
		return
	}
	switch reply.Response {
	case "allow":
		ch <- agent.PermAllow
	case "allowAlways":
		ch <- agent.PermAllowAlways
	default:
		ch <- agent.PermDeny
	}
}

func (s *Session) resolveAsk(reply askUserReply) {
	s.pendingMu.Lock()
	ch, ok := s.pendingAsks[reply.ID]
	delete(s.pendingAsks, reply.ID)
	delete(s.pendingEvent, reply.ID)
	s.pendingMu.Unlock()
	if !ok {
		return
	}
	ch <- tools.QuestionResponse{Answers: reply.Answers}
}

func (s *Session) beginRun(cancel context.CancelFunc) {
	s.stateMu.Lock()
	s.streaming = true
	s.cancelRun = cancel
	s.cancelled = false
	s.lastActive = time.Now()
	s.stateMu.Unlock()
}

func (s *Session) endRun() {
	s.stateMu.Lock()
	s.streaming = false
	s.cancelRun = nil
	s.lastActive = time.Now()
	s.stateMu.Unlock()
}

func (s *Session) cancel() {
	s.stateMu.Lock()
	cancel := s.cancelRun
	if cancel != nil {
		s.cancelled = true
	}
	s.stateMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Session) cancelWasRequested() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.cancelled
}

func (s *Session) setAnchor(id string) {
	s.stateMu.Lock()
	s.anchorID = id
	s.lastActive = time.Now()
	s.stateMu.Unlock()
}

func (s *Session) recordUsage(in, out int) {
	s.stateMu.Lock()
	s.lastUsage = [2]int{in, out}
	s.stateMu.Unlock()
}

func (s *Session) snapshot() (streaming, ready bool, anchor string, usage [2]int) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.streaming, s.ready, s.anchorID, s.lastUsage
}

func (s *Session) idleFor() time.Duration {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return time.Since(s.lastActive)
}

// attach registers a client and brings it up to date: current run state, the
// command menu, and any prompt still waiting on an answer.
func (s *Session) attach(ws *swifty_http.WSConn) {
	s.connMu.Lock()
	s.conns[ws] = struct{}{}
	s.connMu.Unlock()

	streaming, ready, anchor, usage := s.snapshot()
	s.sendTo(ws, Event{Type: "connected", Data: map[string]any{
		"model":          s.provider.Model,
		"streaming":      streaming,
		"ready":          ready,
		"anchorId":       anchor,
		"inputTokens":    usage[0],
		"outputTokens":   usage[1],
		"permissionMode": string(s.ag.Checker.Mode),
	}})
	s.sendTo(ws, Event{Type: "commands", Data: s.commandList()})

	s.pendingMu.Lock()
	pending := make([]Event, 0, len(s.pendingEvent))
	for _, ev := range s.pendingEvent {
		pending = append(pending, ev)
	}
	s.pendingMu.Unlock()
	for _, ev := range pending {
		s.sendTo(ws, ev)
	}
}

func (s *Session) detach(ws *swifty_http.WSConn) {
	s.connMu.Lock()
	delete(s.conns, ws)
	s.connMu.Unlock()
	s.stateMu.Lock()
	s.lastActive = time.Now()
	s.stateMu.Unlock()
}

func (s *Session) hasConns() bool {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return len(s.conns) > 0
}

// emit fans an event out to every device the user has open. Events are pure UI
// progress, so a dead connection is logged and skipped rather than retried.
func (s *Session) emit(ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("agent_hub %s: marshal %s failed: %v", s.userID, ev.Type, err)
		return
	}
	s.connMu.Lock()
	conns := make([]*swifty_http.WSConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.connMu.Unlock()
	for _, c := range conns {
		if err := c.WriteMessage(swifty_http.TextMessage, data); err != nil {
			log.Printf("agent_hub %s: write %s failed: %v", s.userID, ev.Type, err)
		}
	}
}

func (s *Session) sendTo(ws *swifty_http.WSConn, ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if err := ws.WriteMessage(swifty_http.TextMessage, data); err != nil {
		log.Printf("agent_hub %s: write %s failed: %v", s.userID, ev.Type, err)
	}
}

func (s *Session) close() {
	s.stopOnce.Do(func() {
		close(s.done)
		s.cancel()
	})
}

// SkillHost implementation, letting the Skill tool narrow this agent's tools.

func (s *Session) ActivateSkill(name, body string) { s.ag.ActivateSkill(name, body) }

func (s *Session) SetToolFilter(allow func(name string) bool) { s.ag.SetToolFilter(allow) }

func (s *Session) ToolRegistry() *tools.Registry { return s.registry }

// Helpers

func resolveMode(mode string) permissions.PermissionMode {
	switch permissions.PermissionMode(mode) {
	case permissions.ModeAcceptEdits:
		return permissions.ModeAcceptEdits
	case permissions.ModeBypass:
		return permissions.ModeBypass
	case permissions.ModePlan:
		return permissions.ModePlan
	default:
		return permissions.ModeDefault
	}
}

func buildInstructions(wd string) string {
	parts := []string{}
	for _, p := range []string{
		filepath.Join(wd, ".swiftx", "instructions.md"),
		filepath.Join(wd, "AGENTS.md"),
	} {
		if data, err := os.ReadFile(p); err == nil {
			parts = append(parts, string(data))
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildSkillSection(catalog *skills.Catalog, wd string) string {
	if catalog == nil {
		return ""
	}
	metas := catalog.List()
	if len(metas) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Available Skills\n\n")
	fmt.Fprintf(&sb, "Skills are installed at: %s\n", filepath.Join(wd, ".swiftx", "skills"))
	sb.WriteString("When creating new skills, always place them under this directory as <skill-name>/SKILL.md.\n\n")
	for _, meta := range metas {
		desc := meta.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		fmt.Fprintf(&sb, "- /%s: %s\n", meta.Name, desc)
	}
	return sb.String()
}

// installMemExtractor wires background memory extraction. UserMemoryDir is left
// empty on purpose: ~/.swiftx/memory is shared by the whole process, so writing
// there would leak one chat user's memories into every other user's agent.
func installMemExtractor(ag *agent.Agent, wd, protocol string, client llm.Client, registry *tools.Registry, conv *conversation.Manager) *extractor.Extractor {
	extr := extractor.InitExtractMemories(extractor.Deps{
		MemoryDir:    memory.GetAutoMemPath(wd),
		ProjectRoot:  wd,
		Client:       client,
		ToolRegistry: registry,
		Protocol:     protocol,
		Conversation: conv,
		AppendSystem: func(s string) { conv.AddSystemReminder(s) },
	})
	ag.OnLoopComplete = func(_ *conversation.Manager) {
		_ = extr.Execute(context.Background())
	}
	return extr
}

func newAgentHookRunner(client llm.Client) func(prompt string, ctx hooks.HookContext) (string, error) {
	return func(p string, _ hooks.HookContext) (string, error) {
		c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		conv := conversation.NewManager()
		conv.AddUserMessage(p)
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

// compactConversation is the /compact implementation, split out so the command
// switch stays readable.
func (s *Session) compactConversation() {
	s.emit(Event{Type: "system", Data: map[string]string{"message": "Compacting conversation…"}})
	var recovery *compact.RecoveryState
	var schemas []map[string]any
	if s.ag != nil {
		recovery = s.ag.RecoveryState
		schemas = s.ag.Registry.GetAllSchemas(s.ag.Protocol)
	}
	msg, err := compact.ForceCompact(context.Background(), s.conv, s.client, s.workDir,
		s.sessionID, s.provider.GetContextWindow(), recovery, schemas)
	if err != nil {
		s.emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return
	}
	s.emit(Event{Type: "system", Data: map[string]string{"message": "⟳ " + msg}})
}

func (s *Session) planPath() string { return plan_file.GetOrCreatePlanPath(s.workDir) }
