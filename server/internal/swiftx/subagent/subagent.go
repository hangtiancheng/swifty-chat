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

package subagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

type Task struct {
	ID        string
	Name      string
	Status    TaskStatus
	Output    string
	Error     string
	CreatedAt time.Time
	DoneAt    time.Time
	Cancel    context.CancelFunc
}

type TaskManager struct {
	mu            sync.Mutex
	tasks         map[string]*Task
	nextID        int
	notifications []TaskNotification
}

type TaskNotification struct {
	TaskID string
	Name   string
	Status TaskStatus
	Output string
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*Task),
	}
}

func (tm *TaskManager) CreateTask(name string) string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.nextID++
	id := fmt.Sprintf("task_%d", tm.nextID)
	tm.tasks[id] = &Task{
		ID:        id,
		Name:      name,
		Status:    TaskPending,
		CreatedAt: time.Now(),
	}
	return id
}

func (tm *TaskManager) GetTask(id string) *Task {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.tasks[id]
}

func (tm *TaskManager) ListTasks() []*Task {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	var result []*Task
	for _, t := range tm.tasks {
		result = append(result, t)
	}
	return result
}

func (tm *TaskManager) SetRunning(id string, cancel context.CancelFunc) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[id]; ok {
		t.Status = TaskRunning
		t.Cancel = cancel
	}
}

func (tm *TaskManager) SetCompleted(id, output string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[id]; ok {
		t.Status = TaskCompleted
		t.Output = output
		t.DoneAt = time.Now()
		tm.notifications = append(tm.notifications, TaskNotification{
			TaskID: id,
			Name:   t.Name,
			Status: TaskCompleted,
			Output: output,
		})
	}
}

func (tm *TaskManager) SetFailed(id, errMsg string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[id]; ok {
		t.Status = TaskFailed
		t.Error = errMsg
		t.DoneAt = time.Now()
		tm.notifications = append(tm.notifications, TaskNotification{
			TaskID: id,
			Name:   t.Name,
			Status: TaskFailed,
			Output: errMsg,
		})
	}
}

func (tm *TaskManager) DrainNotifications() []TaskNotification {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	n := tm.notifications
	tm.notifications = nil
	return n
}

func (tm *TaskManager) AdoptRunning(name string, eventCh <-chan agent.AgentEvent, cancel context.CancelFunc) string {
	taskID := tm.CreateTask("adopted: " + truncate(name, 40))
	tm.SetRunning(taskID, cancel)

	go func() {
		var output strings.Builder
		for ev := range eventCh {
			switch e := ev.(type) {
			case agent.StreamText:
				output.WriteString(e.Text)
			case agent.ErrorEvent:
				tm.SetFailed(taskID, e.Message)
				return
			}
		}
		tm.SetCompleted(taskID, output.String())
	}()

	return taskID
}

func (tm *TaskManager) FindByName(name string) *Task {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, t := range tm.tasks {
		if t.Name == name || strings.HasPrefix(t.Name, name+":") {
			return t
		}
	}
	return nil
}

func (tm *TaskManager) CancelTask(id string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[id]; ok && t.Cancel != nil {
		t.Cancel()
		t.Status = TaskCancelled
		t.DoneAt = time.Now()
		return true
	}
	return false
}

// SubAgentSpec captures the runtime-relevant subset of BaseAgentDefinition. It is the bridge
// between the load layer (AgentDefinition) and the execution layer (runSync / runAsync / runFork).
// Built-in agents skip file parsing and instantiate this directly via BuiltinSpecs.
type SubAgentSpec struct {
	Name                 string
	Description          string
	Tools                []string
	DisallowedTools      []string
	SystemPromptOverride string
	MaxTurns             int
	Model                string

	// PermissionMode overrides the parent agent's permission mode while the sub-agent runs. Empty
	// string means inherit from parent.
	PermissionMode string

	// Background forces this agent to run as a background task when spawned, regardless of the
	// run_in_background call-site parameter.
	Background bool

	// Isolation selects a file-system isolation mode; "worktree" creates a temporary git worktree.
	Isolation IsolationMode

	// InitialPrompt is prepended to the first user turn.
	InitialPrompt string

	// OmitSwiftxMd drops the SWIFTX.md hierarchy from this agent's userContext.
	OmitSwiftxMd bool

	// Skills are skill names to preload when the sub-agent starts.
	Skills []string

	// Memory enables persistent memory in one of three scopes.
	Memory AgentMemoryScope

	// McpServers / RequiredMcpServers / Hooks / Effort carry frontmatter data forward so future
	// channels can consume it without another schema migration.
	McpServers         []any
	RequiredMcpServers []string
	Hooks              any
	Effort             any
}

const planAgentSystemPrompt = `You are a software architect and planning specialist.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
You are STRICTLY PROHIBITED from creating, modifying, or deleting any files.
Your role is EXCLUSIVELY to explore code and design implementation plans.

## Your Process

1. **Understand Requirements**: Analyze the user's request carefully.

2. **Explore Thoroughly**:
   - Read files with ReadFile to understand current architecture
   - Use Grep to find patterns, function definitions, and references
   - Use Glob to discover file structure
   - Use Bash ONLY for read-only operations (ls, find, grep, cat, head, tail)
   - NEVER use Bash for: mkdir, touch, rm, cp, mv, git add/commit, npm install

3. **Design Solution**:
   - Create a concrete implementation approach
   - Consider trade-offs and explain your reasoning
   - Follow existing patterns in the codebase

4. **Detail the Plan**:
   - Provide step-by-step implementation strategy
   - Identify file dependencies and sequencing
   - Anticipate potential challenges

## Required Output
End your response with:

### Critical Files for Implementation
List the most critical files for implementing this change:
- path/to/file1 — reason
- path/to/file2 — reason`

var BuiltinSpecs = map[string]SubAgentSpec{
	"general-purpose": {
		Name:        "general-purpose",
		Description: "General-purpose agent for research and multi-step tasks",
		MaxTurns:    200,
	},
	"plan": {
		Name:                 "plan",
		Description:          "Software architect for designing implementation plans. Returns step-by-step plans, identifies critical files, and considers architectural trade-offs.",
		DisallowedTools:      []string{"EditFile", "WriteFile"},
		SystemPromptOverride: planAgentSystemPrompt,
		MaxTurns:             15,
	},
	"explore": {
		Name:            "explore",
		Description:     "Fast read-only search agent for locating code",
		DisallowedTools: []string{"EditFile", "WriteFile"},
		// MaxTurns omitted → defaults to 200 (same fallback as general-purpose). Previous 30-turn cap was
		// tripping when the LLM had to issue many ToolSearch/Glob/Grep calls to map an unfamiliar repo,
		// causing the spawn to fail with "reached maximum iterations" before it could report anything
		// useful.
		Model: "haiku",
	},
}

func SpawnSubAgent(
	ctx context.Context,
	taskMgr *TaskManager,
	client llm.Client,
	registry *tools.Registry,
	protocol string,
	spec SubAgentSpec,
	taskPrompt string,
	parentChecker *permissions.Checker,
) string {
	taskID := taskMgr.CreateTask(spec.Name + ": " + truncate(taskPrompt, 50))

	subCtx, cancel := context.WithCancel(ctx)
	taskMgr.SetRunning(taskID, cancel)

	subRegistry := FilterToolsForAgent(registry, spec.Tools, spec.DisallowedTools, true)

	subAgent := agent.New(client, subRegistry, protocol)
	subAgent.Checker = deriveSubAgentChecker(parentChecker, spec.PermissionMode)
	if spec.MaxTurns > 0 {
		subAgent.MaxIterations = spec.MaxTurns
	} else {
		subAgent.MaxIterations = 200
	}

	go func() {
		conv := conversation.NewManager()
		if spec.SystemPromptOverride != "" {
			conv.AddSystemReminder(spec.SystemPromptOverride)
		}
		// initialPrompt is prepended to the first user turn.
		if spec.InitialPrompt != "" {
			conv.AddUserMessage(spec.InitialPrompt)
		}
		conv.AddUserMessage(taskPrompt)

		var output strings.Builder
		ch := subAgent.Run(subCtx, conv)
		for ev := range ch {
			switch e := ev.(type) {
			case agent.StreamText:
				output.WriteString(e.Text)
			case agent.PermissionRequestEvent:
				// Background sub-agents are headless: auto-deny so executeSingleTool doesn't stall on respCh
				// forever.
				e.ResponseCh <- agent.PermDeny
			case agent.ErrorEvent:
				taskMgr.SetFailed(taskID, e.Message)
				return
			case agent.LoopComplete:
				// done
			}
		}
		taskMgr.SetCompleted(taskID, output.String())
	}()

	return taskID
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
