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

// Package consolidation implements background memory consolidation (autoDream).
//
// Automatically triggered when two gate conditions are met: more than minHours
// have elapsed since the last consolidation, and at least minSessions sessions
// have accumulated during that period. Once triggered, a sub-agent is forked in
// the background to scan existing memories, merge duplicates, prune stale entries,
// fix contradictions, and maintain the index.
package consolidation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/subagent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

const (
	defaultMinHours    = 24
	defaultMinSessions = 5
	// scanThrottleMs prevents scanning the session directory every round when the time gate passes but the session gate does not
	scanThrottleMs = 10 * 60 * 1000
)

// Deps holds the external dependencies for Consolidator.
type Deps struct {
	MemoryDir     string                // <wd>/.swiftx/memory/
	UserMemoryDir string                // ~/.swiftx/memory/
	ProjectRoot   string                // absolute path to the project root
	Client        llm.Client            // LLM client
	ToolRegistry  *tools.Registry       // parent agent's tool registry
	Protocol      string                // "anthropic" / "openai"
	Conversation  *conversation.Manager // parent agent's conversation
	AppendSystem  func(string)          // notify TUI
	DebugLogf     func(string, ...any)  // debug logging
}

// Consolidator manages the state and execution of background memory consolidation.
type Consolidator struct {
	deps        Deps
	lastScanAt  int64 // timestamp of last session directory scan (ms)
	minHours    int
	minSessions int
}

// NewConsolidator creates a new consolidator.
func NewConsolidator(deps Deps) *Consolidator {
	return &Consolidator{
		deps:        deps,
		minHours:    defaultMinHours,
		minSessions: defaultMinSessions,
	}
}

// SetThresholds overrides the default gate thresholds (for testing).
func (c *Consolidator) SetThresholds(minHours, minSessions int) {
	c.minHours = minHours
	c.minSessions = minSessions
}

// MaybeRun checks gate conditions; if met, executes one background consolidation pass.
// Called after each agent loop completes; very low cost (one stat call).
func (c *Consolidator) MaybeRun(ctx context.Context) {
	if c == nil {
		return
	}
	// Skip if memory directory does not exist
	if _, err := os.Stat(strings.TrimRight(c.deps.MemoryDir, string(filepath.Separator))); os.IsNotExist(err) {
		return
	}

	// Time gate: has enough time elapsed since the last consolidation?
	lastAt, err := ReadLastConsolidatedAt(c.deps.MemoryDir)
	if err != nil {
		c.debugf("[consolidation] ReadLastConsolidatedAt failed: %v", err)
		return
	}
	hoursSince := float64(time.Now().UnixMilli()-lastAt) / 3_600_000
	if hoursSince < float64(c.minHours) {
		return
	}

	// Scan throttle: prevent scanning the session directory every round
	now := time.Now().UnixMilli()
	if now-c.lastScanAt < scanThrottleMs {
		c.debugf("[consolidation] scan throttle — last scan %ds ago", (now-c.lastScanAt)/1000)
		return
	}
	c.lastScanAt = now

	// Session gate: has enough sessions accumulated to reach the threshold?
	sessionIDs := listSessionsSince(c.deps.ProjectRoot, lastAt)
	if len(sessionIDs) < c.minSessions {
		c.debugf("[consolidation] skip — %d sessions since last consolidation, need %d",
			len(sessionIDs), c.minSessions)
		return
	}

	// Acquire lock
	priorMtime, err := TryAcquireLock(c.deps.MemoryDir)
	if err != nil {
		c.debugf("[consolidation] lock acquire failed: %v", err)
		return
	}
	if priorMtime == -1 {
		c.debugf("[consolidation] lock held by another process")
		return
	}

	c.debugf("[consolidation] firing — %.1fh since last, %d sessions to review",
		hoursSince, len(sessionIDs))

	go c.run(ctx, sessionIDs, priorMtime)
}

func (c *Consolidator) run(ctx context.Context, sessionIDs []string, priorMtime int64) {
	defer func() {
		if r := recover(); r != nil {
			c.debugf("[consolidation] panic: %v", r)
			RollbackLock(c.deps.MemoryDir, priorMtime)
		}
	}()

	transcriptDir := filepath.Join(c.deps.ProjectRoot, ".swiftx", "sessions")
	prompt := BuildConsolidationPrompt(
		c.deps.MemoryDir, c.deps.UserMemoryDir,
		transcriptDir, sessionIDs,
	)

	// Build an independent conversation: do not inherit parent agent context, start from a blank slate
	conv := conversation.NewManager()
	conv.AddUserMessage(prompt)

	// Tool filter: give the consolidation sub-agent the async-level tool set
	subRegistry := subagent.FilterToolsForAgent(c.deps.ToolRegistry, nil, nil, true)

	subChecker := memory.NewSubAgentChecker(c.deps.ProjectRoot, c.deps.UserMemoryDir)

	subAgent := agent.New(c.deps.Client, subRegistry, c.deps.Protocol)
	subAgent.MaxIterations = 15 // consolidation may require multiple read/write rounds
	subAgent.Checker = subChecker
	subAgent.WorkDir = c.deps.ProjectRoot

	startTime := time.Now()
	ch := subAgent.Run(ctx, conv)
	for range ch {
		// drain
	}

	writtenPaths := extractWrittenPaths(conv.GetMessages())
	c.debugf("[consolidation] finished in %s, %d files touched: %v",
		time.Since(startTime), len(writtenPaths), writtenPaths)

	// Filter out the index file, only notify actual memory file modifications
	var memoryPaths []string
	for _, p := range writtenPaths {
		if filepath.Base(p) == memory.AutoMemEntrypointName {
			continue
		}
		memoryPaths = append(memoryPaths, p)
	}

	if len(memoryPaths) > 0 && c.deps.AppendSystem != nil {
		var names []string
		for _, p := range memoryPaths {
			names = append(names, filepath.Base(p))
		}
		c.deps.AppendSystem(fmt.Sprintf("Memory improved: %s", strings.Join(names, ", ")))
	}
}

// listSessionsSince returns session IDs modified after sinceMs.
func listSessionsSince(projectRoot string, sinceMs int64) []string {
	sessions := session.ListSessions(projectRoot)
	since := time.UnixMilli(sinceMs)
	var ids []string
	for _, s := range sessions {
		if s.ModTime.After(since) {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// extractWrittenPaths extracts all Write/Edit file paths from the sub-agent's conversation.
func extractWrittenPaths(messages []conversation.Message) []string {
	var paths []string
	seen := make(map[string]struct{})
	for _, m := range messages {
		if m.Role != "assistant" {
			continue
		}
		for _, tu := range m.ToolUses {
			if tu.ToolName != "WriteFile" && tu.ToolName != "EditFile" {
				continue
			}
			fp, ok := tu.Arguments["file_path"].(string)
			if !ok || fp == "" {
				continue
			}
			if _, exists := seen[fp]; exists {
				continue
			}
			seen[fp] = struct{}{}
			paths = append(paths, fp)
		}
	}
	return paths
}

func (c *Consolidator) debugf(format string, args ...any) {
	if c.deps.DebugLogf != nil {
		c.deps.DebugLogf(format, args...)
	}
}
