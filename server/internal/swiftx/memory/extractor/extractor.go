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

// Package extractor implements the background memory extraction subagent.
//
// The TS file uses a closure-scoped state pattern (initExtractMemories returns nothing, but mutates
// module-level extractor/drainer pointers); the Go port wraps the same state in an Extractor struct
// + sync.Mutex so each caller gets an independent instance and tests can replace Deps cleanly.
//
// Triggering: TUI sets agent.Agent.OnLoopComplete to a closure that calls (*Extractor).Execute. The
// agent loop fires that callback fire-and-forget after each LoopComplete event. Extractor itself
// spawns its own goroutine stack via runExtraction, which means Execute returns quickly and the
// actual fork happens in the background.
package extractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/subagent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// Deps holds the external collaborators an Extractor needs. The TUI constructs one Deps value at
// startup, wraps it in an Extractor, and hooks the resulting Execute method onto
// agent.Agent.OnLoopComplete.
//
// AppendSystem is the conduit for the "Memory saved: foo.md" notice that the user sees after a
// successful extraction.
type Deps struct {
	MemoryDir     string                           // <wd>/.swiftx/memory/ — project/reference (trailing sep)
	UserMemoryDir string                           // ~/.swiftx/memory/ — user/feedback (trailing sep); may be "" if $HOME unresolved
	ProjectRoot   string                           // absolute project root
	Client        llm.Client                       // forked extraction agent's LLM client
	ToolRegistry  *tools.Registry                  // parent tool registry (will be filtered)
	Protocol      string                           // "anthropic" / "openai"
	Conversation  *conversation.Manager            // parent conversation reference
	AppendSystem  func(string)                     // optional: notify TUI of saved memories
	DebugLogf     func(format string, args ...any) // optional: debug logging
}

// Extractor is the ch09 background memory extractor. State is encapsulated in struct fields; mu
// guards every mutable field. Each Extractor instance is independent — tests can construct one with
// mock Deps without touching global state.
//
// Translated from the TS initExtractMemories closure. Mapping:
// inFlightExtractions Set → inFlight map[*sync.WaitGroup]struct{}
// lastMemoryMessageUuid string|undefined → lastMemoryMessageIdx int
// (Swiftx messages have no uuid; cursor is the parent conversation's
// message-array index at the time the last successful extraction ran)
// hasLoggedGateFailure / inProgress / turnsSinceLastExtraction → bool/int
// pendingContext → *pendingExtractionCtx
type Extractor struct {
	deps Deps

	mu                       sync.Mutex
	inFlight                 map[*sync.WaitGroup]struct{}
	lastMemoryMessageIdx     int
	hasLoggedGateFailure     bool
	inProgress               bool
	turnsSinceLastExtraction int
	pendingContext           *pendingExtractionCtx
}

// pendingExtractionCtx is the stash slot for a trailing extraction. The TS port carries (context,
// appendSystemMessage); the Go port has all state on the Extractor itself, so the stash needs no
// payload — the presence of a non-nil value means "run another extraction when the current one
// finishes".
type pendingExtractionCtx struct{}

// InitExtractMemories constructs a new Extractor with the given Deps.initExtractMemories factory.
func InitExtractMemories(deps Deps) *Extractor {
	return &Extractor{
		deps:     deps,
		inFlight: make(map[*sync.WaitGroup]struct{}),
	}
}

// Execute is the fire-and-forget entrypoint. Wired onto agent.Agent.OnLoopComplete by the TUI.
// Returns quickly; the actual extraction work happens on caller goroutine. Errors are swallowed
// (best-effort) — the caller (agent loop) ignores the return value.
func (e *Extractor) Execute(ctx context.Context) error {
	if e == nil {
		return nil
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	e.mu.Lock()
	e.inFlight[wg] = struct{}{}
	e.mu.Unlock()
	defer func() {
		wg.Done()
		e.mu.Lock()
		delete(e.inFlight, wg)
		e.mu.Unlock()
	}()

	return e.executeImpl(ctx)
}

func (e *Extractor) executeImpl(ctx context.Context) error {
	// In-progress coalescing: if another extraction is currently running, stash this call as a pending
	// trailing run and return immediately.pendingContext / runExtraction.finally trailing chain.
	e.mu.Lock()
	if e.inProgress {
		e.deps.debugf("[extractMemories] extraction in progress — stashing for trailing run")
		e.pendingContext = &pendingExtractionCtx{}
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	return e.runExtraction(ctx, false)
}

func (e *Extractor) runExtraction(ctx context.Context, isTrailingRun bool) error {
	messages := e.deps.Conversation.GetMessages()
	newMessageCount := countModelVisibleMessagesSince(messages, e.lastMemoryMessageIdx)

	// Mutual exclusion: when the main agent wrote memories itself, the forked extractor is redundant.
	// Advance the cursor past this range and return.
	if hasMemoryWritesSince(messages, e.lastMemoryMessageIdx, e.deps.ProjectRoot) {
		e.deps.debugf("[extractMemories] skipping — conversation already wrote to memory files")
		e.advanceCursor(len(messages))
		return nil
	}

	// Throttle: default is 1 (run every turn). Swiftx hardcodes 1; trailing runs bypass the throttle
	// since they process already-committed work.
	if !isTrailingRun {
		e.turnsSinceLastExtraction++
		if e.turnsSinceLastExtraction < 1 {
			return nil
		}
	}
	e.turnsSinceLastExtraction = 0

	e.mu.Lock()
	e.inProgress = true
	e.mu.Unlock()
	startTime := time.Now()

	defer func() {
		e.mu.Lock()
		e.inProgress = false
		trailing := e.pendingContext
		e.pendingContext = nil
		e.mu.Unlock()
		if trailing != nil {
			e.deps.debugf("[extractMemories] running trailing extraction for stashed context")
			_ = e.runExtraction(ctx, true)
		}
	}()

	e.deps.debugf("[extractMemories] starting — %d new messages, memoryDir=%s, userMemoryDir=%s",
		newMessageCount, e.deps.MemoryDir, e.deps.UserMemoryDir)

	// Pre-inject the memory-directory manifest so the extraction agent doesn't burn a turn on `ls`.
	// Scans both directories so user-level and project-level memories appear in one combined manifest.
	var combinedScan []memory.MemoryHeader
	if e.deps.UserMemoryDir != "" {
		userScan, _ := memory.ScanMemoryFiles(ctx, e.deps.UserMemoryDir, "user")
		combinedScan = append(combinedScan, userScan...)
	}
	projectScan, _ := memory.ScanMemoryFiles(ctx, e.deps.MemoryDir, "project")
	combinedScan = append(combinedScan, projectScan...)
	manifest := memory.FormatMemoryManifest(combinedScan)
	extractionPrompt := BuildExtractAutoOnlyPrompt(newMessageCount, manifest, false, e.deps.UserMemoryDir, e.deps.MemoryDir)

	// Build the forked conversation: copy parent messages, then append the extraction prompt as a new
	// user message. Deliberately does NOT add agents.runFork's fork boilerplate — the extractor is a
	// "perfect fork" of the main conversation, we don't inject extra system instructions either.
	forkedConv := buildExtractorConversation(e.deps.Conversation, extractionPrompt)

	// Tool whitelist: ReadFile / WriteFile / EditFile / Glob / Grep / Bash / ToolSearch (via
	// FilterToolsForAgent's async path). Agent and AskUserQuestion are auto-excluded.
	subRegistry := subagent.FilterToolsForAgent(e.deps.ToolRegistry, nil, nil, true)

	// Strict path sandbox: only memoryDir is allowed for file tools. This is stricter than the
	// original createAutoMemCanUseTool (which lets Read/Grep/Glob roam unrestricted) but matches the
	// prompt's explicit warning against grepping source code, so the behavioral impact is small and
	// the safety win is meaningful.
	//
	// Mode = ModeBypass so file/command tools never hit an Ask path — the extractor runs in the
	// background with no TUI to answer.
	subChecker := memory.NewSubAgentChecker(e.deps.ProjectRoot, e.deps.UserMemoryDir)

	subAgent := agent.New(e.deps.Client, subRegistry, e.deps.Protocol)
	subAgent.MaxIterations = 5
	subAgent.Checker = subChecker
	subAgent.WorkDir = e.deps.ProjectRoot

	// Drive the forked agent to completion. Drain the event channel so the loop exits cleanly; we
	// don't surface streaming text — only the file writes matter.
	ch := subAgent.Run(ctx, forkedConv)
	for range ch {
		// drain; do not propagate sub-agent events to the UI.
	}

	// Advance the cursor only after the run completes (whether it wrote files or not — a "ran but
	// picked nothing" turn shouldn't be reconsidered).
	e.advanceCursor(len(messages))

	writtenPaths := extractWrittenPaths(forkedConv.GetMessages())
	e.deps.debugf("[extractMemories] finished in %s, %d files written: %v",
		time.Since(startTime), len(writtenPaths), writtenPaths)

	// Index files (MEMORY.md) are mechanical — the user-visible "memory" is the topic file, not the
	// index update.
	var memoryPaths []string
	for _, p := range writtenPaths {
		if filepath.Base(p) == memory.AutoMemEntrypointName {
			continue
		}
		memoryPaths = append(memoryPaths, p)
	}

	if len(memoryPaths) > 0 && e.deps.AppendSystem != nil {
		var names []string
		for _, p := range memoryPaths {
			names = append(names, filepath.Base(p))
		}
		e.deps.AppendSystem(fmt.Sprintf("Memory saved: %s", strings.Join(names, ", ")))
	}

	return nil
}

// Drain waits for all in-flight extractions (including any pending trailing run) to finish, with a
// soft timeout. Call from the TUI's shutdown path so the forked extraction agent isn't killed mid-
// write.
//
// timeoutMs of 0 returns immediately if any work is still in-flight. Negative timeoutMs is treated
// as 60000 (60s default).
func (e *Extractor) Drain(timeoutMs int) error {
	if e == nil {
		return nil
	}
	if timeoutMs < 0 {
		timeoutMs = 60000
	}

	e.mu.Lock()
	wgs := make([]*sync.WaitGroup, 0, len(e.inFlight))
	for wg := range e.inFlight {
		wgs = append(wgs, wg)
	}
	e.mu.Unlock()

	if len(wgs) == 0 {
		return nil
	}

	done := make(chan struct{})
	go func() {
		for _, wg := range wgs {
			wg.Wait()
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return nil
	}
}

func (e *Extractor) advanceCursor(to int) {
	e.mu.Lock()
	if to > e.lastMemoryMessageIdx {
		e.lastMemoryMessageIdx = to
	}
	e.mu.Unlock()
}

// countModelVisibleMessagesSince counts user/assistant messages added since sinceIdx. Falls back to
// counting all model-visible messages when sinceIdx is out of range (e.g., the conversation was
// compacted and the cursor we recorded no longer maps to a current position)."if !foundStart"
// recovery path.
func countModelVisibleMessagesSince(messages []conversation.Message, sinceIdx int) int {
	if sinceIdx < 0 || sinceIdx > len(messages) {
		return countModelVisible(messages)
	}
	n := 0
	for _, m := range messages[sinceIdx:] {
		if isModelVisibleMessage(m) {
			n++
		}
	}
	return n
}

func countModelVisible(messages []conversation.Message) int {
	n := 0
	for _, m := range messages {
		if isModelVisibleMessage(m) {
			n++
		}
	}
	return n
}

func isModelVisibleMessage(m conversation.Message) bool {
	return m.Role == "user" || m.Role == "assistant"
}

// hasMemoryWritesSince returns true if any assistant message after sinceIdx contains a Write/Edit
// tool_use targeting an auto-memory path. When this returns true, runExtraction skips the agent
// fork.
func hasMemoryWritesSince(messages []conversation.Message, sinceIdx int, projectRoot string) bool {
	if sinceIdx < 0 {
		sinceIdx = 0
	}
	if sinceIdx >= len(messages) {
		return false
	}
	for _, m := range messages[sinceIdx:] {
		if m.Role != "assistant" {
			continue
		}
		for _, tu := range m.ToolUses {
			fp := getWrittenFilePath(tu)
			if fp == "" {
				continue
			}
			if memory.IsAutoMemPath(fp, projectRoot) {
				return true
			}
		}
	}
	return false
}

// getWrittenFilePath extracts the file_path argument from a Write/Edit tool_use block, or "" if the
// block is not such a call.
func getWrittenFilePath(tu conversation.ToolUseBlock) string {
	if tu.ToolName != "WriteFile" && tu.ToolName != "EditFile" {
		return ""
	}
	fp, ok := tu.Arguments["file_path"].(string)
	if !ok {
		return ""
	}
	return fp
}

// extractWrittenPaths collects unique file_path arguments from every Write/Edit tool_use across the
// forked agent's assistant messages. First occurrence wins.
func extractWrittenPaths(messages []conversation.Message) []string {
	var paths []string
	seen := make(map[string]struct{})
	for _, m := range messages {
		if m.Role != "assistant" {
			continue
		}
		for _, tu := range m.ToolUses {
			fp := getWrittenFilePath(tu)
			if fp == "" {
				continue
			}
			if _, ok := seen[fp]; ok {
				continue
			}
			seen[fp] = struct{}{}
			paths = append(paths, fp)
		}
	}
	return paths
}

// buildExtractorConversation copies the parent conversation's messages into a new Manager and
// appends the extraction prompt as a final user message. Unlike agents.buildForkedConversation, it
// does NOT inject the ForkBoilerplateTag — the original extractor is a perfect fork, not a "you are
// a subagent" fork.
func buildExtractorConversation(parent *conversation.Manager, prompt string) *conversation.Manager {
	forked := conversation.NewManager()
	for _, msg := range parent.GetMessages() {
		switch msg.Role {
		case "assistant":
			if len(msg.ToolUses) > 0 {
				forked.AddAssistantMessageWithTools(msg.Content, msg.ToolUses)
			} else {
				forked.AddAssistantMessage(msg.Content)
			}
		default:
			if len(msg.ToolResults) > 0 {
				forked.AddToolResultsMessage(msg.ToolResults)
			} else {
				forked.AddUserMessage(msg.Content)
			}
		}
	}
	forked.AddUserMessage(prompt)
	return forked
}

func (d Deps) debugf(format string, args ...any) {
	if d.DebugLogf != nil {
		d.DebugLogf(format, args...)
	}
}
