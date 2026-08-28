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

package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/compact"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/file_history"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/hooks"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/plan_file"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/prompt"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tool_result"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

const (
	maxTokensCeiling          = 64000
	maxOutputTokensRecoveries = 3
)

type Agent struct {
	Client        llm.Client
	Registry      *tools.Registry
	Protocol      string
	WorkDir       string
	MaxIterations int
	ContextWindow int
	// MaxOutputTokens is the model's max output budget; used by Layer 2 to
	// compute the effective window for the compaction threshold. Zero falls
	// back to the summaryOutputReserve default inside compact.
	MaxOutputTokens int
	Checker         *permissions.Checker
	Hooks           *hooks.Engine
	// SessionID identifies the on-disk session log this agent appends to. When
	// set, Layer 2 compaction writes a compact_boundary record into that session
	// so a later resume can rebuild the compacted state instead of replaying the
	// full pre-compaction transcript. Empty disables boundary persistence (tests,
	// one-shot callers).
	SessionID      string
	NotificationFn func() []string
	// ToolNameFilter, when non-nil, drops any tool whose Name returns false from the schemas sent to
	// the LLM. The filter is consulted at the top of every iteration so callers can flip Coordinator
	// Mode on or off (e.g., when a team is created/torn down) without restarting the agent.
	Instructions  string
	MemoryContent string
	// SkillSection is the listing text of available skills. It is project-scoped,
	// so it goes into the first system-reminder alongside instructions and memory
	// rather than the system prompt.
	SkillSection string
	// SkillDeltaFn returns the listing of skills newly appeared since the last call
	// (previously announced ones are omitted). When skills are installed mid-session,
	// only the new entries are appended — the full listing is not resent and the
	// system prompt is left untouched.
	SkillDeltaFn func() string
	// MemoryRecallCh is a non-blocking memory recall channel: prefetch runs in
	// parallel with the main LLM call; the result is read and injected after
	// tool execution completes.
	MemoryRecallCh <-chan RecallResult
	ToolNameFilter func(name string) bool
	// CoordinatorActiveFn, when non-nil, reports whether Coordinator Mode is currently in effect.
	// Consulted every iteration alongside ToolNameFilter so the scheduling guidance appears exactly
	// when the tool set is narrowed, and disappears once the team is torn down.
	CoordinatorActiveFn func() bool
	// OnLoopComplete, when non-nil, is invoked fire-and-forget after the agent reaches LoopComplete
	// (final assistant message, no tool calls remaining). Used by ch09 background memory extraction.
	// Replaces the original stopHooks dispatcher; failures are silent and must not block the main
	// loop. The callback receives the live conversation — do not mutate it from another goroutine.
	OnLoopComplete  func(conv *conversation.Manager)
	FileHistory     *file_history.History
	compactTracking compact.AutoCompactTrackingState
	// RecoveryState holds the snapshots needed to rebuild working context
	// after Layer 2 collapses the conversation into a summary: most-recent
	// file reads and skill invocations. The struct is concurrency-safe so
	// the streaming executor can write to it from multiple goroutines.
	RecoveryState *compact.RecoveryState
	eventCh       chan AgentEvent
	// activeSkills tracks which Skill SOPs have been activated in this session (name → body).
	// Used by /skills to show what's active and by RecoveryState to survive compaction.
	// The body is injected once into the conversation as a message — NOT re-injected every turn.
	activeSkills map[string]string
	// announcedDeferred is the deferred tool list last announced to the model, in lexicographic
	// order. Comparing it with the current list tells us whether the pool changed; if not,
	// the reminder is not re-sent.
	announcedDeferred []string
	// recallMu guards the two fields below: tool execution writes from multiple
	// goroutines, while memory recall prefetch reads and writes from its own.
	recallMu sync.Mutex
	// recentToolNames holds recently invoked tool names, deduplicated in call order.
	// Passed to the memory recall selector so it skips usage-guide memories for
	// these tools, while still surfacing pitfall and warning memories.
	recentToolNames []string
	// surfacedMemPaths records memory file paths already injected this session;
	// pre-filtered before recall to avoid the same memory occupying a selector
	// slot every turn.
	surfacedMemPaths map[string]struct{}
}

// RecallResult holds the output of a single memory recall pass: the rendered
// system-reminder body and the selected memory file paths. Paths are only
// marked as surfaced once the reminder is actually injected into the conversation.
type RecallResult struct {
	Reminder string
	Paths    []string
}

// maxRecentTools is the upper bound on recent tool names passed to the memory recall selector.
const maxRecentTools = 10

// RecordRecentTool records a just-executed tool name. Re-invoking the same
// tool moves it to the tail, so the list reflects most-recent-use order rather
// than first-use order.
func (a *Agent) RecordRecentTool(name string) {
	if name == "" {
		return
	}
	a.recallMu.Lock()
	defer a.recallMu.Unlock()
	if i := slices.Index(a.recentToolNames, name); i >= 0 {
		a.recentToolNames = slices.Delete(a.recentToolNames, i, i+1)
	}
	a.recentToolNames = append(a.recentToolNames, name)
	if len(a.recentToolNames) > maxRecentTools {
		a.recentToolNames = a.recentToolNames[1:]
	}
}

// RecallHints returns snapshots of the two pieces of state used by memory
// recall: recently used tool names, and memory paths already injected this
// session. The returned values are copies, safe for use in any goroutine.
func (a *Agent) RecallHints() ([]string, map[string]struct{}) {
	a.recallMu.Lock()
	defer a.recallMu.Unlock()
	tools := slices.Clone(a.recentToolNames)
	surfaced := make(map[string]struct{}, len(a.surfacedMemPaths))
	for p := range a.surfacedMemPaths {
		surfaced[p] = struct{}{}
	}
	return tools, surfaced
}

// MarkMemoriesSurfaced records which memories were injected this turn so the
// next recall excludes them first.
func (a *Agent) MarkMemoriesSurfaced(paths []string) {
	if len(paths) == 0 {
		return
	}
	a.recallMu.Lock()
	defer a.recallMu.Unlock()
	if a.surfacedMemPaths == nil {
		a.surfacedMemPaths = make(map[string]struct{}, len(paths))
	}
	for _, p := range paths {
		a.surfacedMemPaths[p] = struct{}{}
	}
}

// deferredReminderMarker is the fixed prefix of the deferred-tool reminder. It is used to check
// whether the reminder still exists in history: after compaction collapses history into a summary
// the original message is gone and must be re-sent.
const deferredReminderMarker = "The following deferred tools are available via ToolSearch."

// ActivateSkill records a skill activation. The body is kept for /skills listing and compaction
// recovery, but is NOT re-injected every turn — it lives in the conversation as a regular message.
func (a *Agent) ActivateSkill(name, body string) {
	if a.activeSkills == nil {
		a.activeSkills = make(map[string]string)
	}
	a.activeSkills[name] = body
}

// ClearActiveSkills drops every pinned SOP. Called by /clear so a fresh conversation doesn't carry
// over SOPs from a prior task. Safe to call when no skills were ever activated.
func (a *Agent) ClearActiveSkills() {
	a.activeSkills = nil
}

// GetActiveSkills returns a copy of the currently-pinned SOPs (name → body). Used by tests and by
// /skills to surface what's active.
func (a *Agent) GetActiveSkills() map[string]string {
	out := make(map[string]string, len(a.activeSkills))
	maps.Copy(out, a.activeSkills)
	return out
}

// SetToolFilter installs a tool visibility filter for the current conversation. The filter is
// consulted at the top of every iteration so callers can flip Coordinator mode on or off without
// restarting the agent. Passing nil clears any previous filter.
func (a *Agent) SetToolFilter(allow func(name string) bool) {
	a.ToolNameFilter = allow
}

// ToolRegistry returns the live tool registry. Named ToolRegistry (not just Registry, even though
// that would match the field name) to avoid the method/field collision Go disallows. Matches the
// skills.SkillHost contract.
func (a *Agent) ToolRegistry() *tools.Registry {
	return a.Registry
}

func New(client llm.Client, registry *tools.Registry, protocol string) *Agent {
	wd, _ := os.Getwd()
	return &Agent{
		Client:        client,
		Registry:      registry,
		Protocol:      protocol,
		WorkDir:       wd,
		MaxIterations: 0,
		ContextWindow: 200000,
		RecoveryState: compact.NewRecoveryState(),
	}
}

// SetSessionID wires the on-disk session log id onto the agent so Layer 2
// compaction can persist a compact_boundary record into the same session the TUI
// is appending plain messages to. Called from the TUI right after the agent is
// constructed (and again after a resume switches sessions).
func (a *Agent) SetSessionID(id string) { a.SessionID = id }

// currentToolSchemas builds the schema list the next API call will use,
// honouring any active ToolNameFilter (e.g. Teams coordinator mode).
// Shared between the recovery attachment (which lists what's still
// available after compact) and the actual Stream call so both views
// stay consistent.
func (a *Agent) currentToolSchemas() []map[string]any {
	schemas := a.Registry.GetAllSchemas(a.Protocol)
	if a.ToolNameFilter == nil {
		return schemas
	}
	// The filter is the sole authority; no exception branches are retained.
	return filterSchemasByName(schemas, a.ToolNameFilter)
}

func (a *Agent) Run(ctx context.Context, conv *conversation.Manager) <-chan AgentEvent {
	ch := make(chan AgentEvent, 32)

	go func() {
		defer close(ch)
		defer a.emitHook(hooks.EventSessionEnd, "", nil)

		a.emitHook(hooks.EventSessionStart, "", nil)

		conv.InjectLongTermMemory(a.Instructions, a.MemoryContent, a.SkillSection)

		var totalInput, totalOutput int
		maxTokensEscalated := false
		outputRecoveries := 0

		for iteration := 1; ; iteration++ {
			if a.MaxIterations > 0 && iteration > a.MaxIterations {
				ch <- ErrorEvent{Message: fmt.Sprintf("Agent reached maximum iterations (%d)", a.MaxIterations)}
				return
			}

			if ctx.Err() != nil {
				return
			}

			// Compute the tool schema list once per iteration so the recovery
			// attachment (when compact fires) and the actual Stream call below
			// agree on what's wired up. Skill filters can only change between
			// iterations, never within one.
			toolSchemas := a.currentToolSchemas()

			// Plan mode: inject structured workflow reminder.
			if a.Checker != nil && a.Checker.Mode == permissions.ModePlan {
				planPath := plan_file.GetOrCreatePlanPath(a.WorkDir)
				a.Checker.PlanFilePath = planPath
				planExists := plan_file.PlanExists(a.WorkDir)
				reminder := prompt.BuildPlanModeReminder(planPath, planExists, iteration)
				conv.AddSystemReminder(reminder)
			}

			// Coordinator Mode: inject scheduling guidance alongside the narrowed tool set.
			// Delivered via system-reminder rather than the system prompt: in long sessions
			// the initial constraints get buried, and appending each turn keeps them salient.
			// The system prompt is a cache prefix — mutating it invalidates the entire cache.
			if a.CoordinatorActiveFn != nil && a.CoordinatorActiveFn() {
				conv.AddSystemReminder(prompt.CoordinatorReminder(iteration))
			}

			if a.NotificationFn != nil {
				for _, note := range a.NotificationFn() {
					conv.AddSystemReminder(note)
				}
			}

			a.emitHook(hooks.EventTurnStart, "", nil)

			// Skills added mid-session: only append the newly appeared entries,
			// without resending the full listing or touching the system prompt,
			// to avoid invalidating the cache prefix.
			if a.SkillDeltaFn != nil {
				if delta := a.SkillDeltaFn(); delta != "" {
					conv.AddSystemReminder("The following skills became available:\n" + delta)
				}
			}

			// Inject deferred tool names into system-reminder so the model knows what's available via
			// ToolSearch. In dispatch mode these tools never enter tools[], so the
			// model must be explicitly told to invoke them via McpCall; otherwise
			// it reads the schema but has no way to call the tool.
			// Sent only when needed, not every turn. The reminder is appended to history, so once
			// sent it stays in context; re-sending identical content each turn would only waste
			// window space: ~60 MCP tools produce a list of 500+ tokens, which adds up to 20k+
			// over 40 turns.
			//
			// Two cases require re-sending: the pool changed (MCP servers connect asynchronously
			// and may reconnect), or the previous reminder was removed by compaction. The latter
			// is detected by scanning history, avoiding the need for a separate hook in the
			// compaction path.
			if deferredNames := a.Registry.GetDeferredToolNames(); len(deferredNames) > 0 {
				poolChanged := !slices.Equal(a.announcedDeferred, deferredNames)
				if poolChanged || !conv.HasReminderContaining(deferredReminderMarker) {
					reminder := deferredReminderMarker + " Their schemas are NOT loaded - use ToolSearch with query \"select:<name>[,<name>...]\" to load tool schemas"
					if a.Registry.McpLoadingMode == tools.McpLoadingDispatch {
						reminder += ", then invoke them with the mcp_call tool"
					} else {
						reminder += " before calling them"
					}
					conv.AddSystemReminder(reminder + ":\n" + strings.Join(deferredNames, "\n"))
					a.announcedDeferred = deferredNames
				}
			}

			a.emitHook(hooks.EventPreSend, "", nil)

			// Layer 2: auto-compact
			// Layer 1 (tool result budget) is already applied when results enter history;
			// the stored content is at its final size, so token estimation uses it directly.
			if msg, err := compact.ManageContext(ctx, conv, a.Client, a.WorkDir, a.SessionID, a.ContextWindow, a.MaxOutputTokens, &a.compactTracking, a.RecoveryState, toolSchemas); err == nil && msg != "" {
				ch <- CompactEvent{Message: msg}
				conv.ClearUsageAnchor()
				conv.InjectLongTermMemory(a.Instructions, a.MemoryContent, a.SkillSection)
			}

			events, errs := a.Client.Stream(ctx, conv, toolSchemas)

			var text strings.Builder
			var toolCalls []llm.ToolCallComplete
			var thinkingBlocks []conversation.ThinkingBlock
			var stopReason string
			var usage llm.UsageInfo

			executor := NewStreamingExecutor(a.Registry, ch)

			for ev := range events {
				switch e := ev.(type) {
				case llm.ThinkingDelta:
					ch <- ThinkingText{Text: e.Text}
				case llm.ThinkingComplete:
					thinkingBlocks = append(thinkingBlocks, conversation.ThinkingBlock{
						Thinking:  e.Thinking,
						Signature: e.Signature,
					})
				case llm.TextDelta:
					text.WriteString(e.Text)
					ch <- StreamText{Text: e.Text}
				case llm.ToolCallStart:
					ch <- ToolUseEvent{ToolID: e.ToolID, ToolName: e.ToolName}
				case llm.ToolCallDelta:
					// ignore
				case llm.ToolCallComplete:
					toolCalls = append(toolCalls, e)
					ch <- ToolUseEvent{
						ToolID:   e.ToolID,
						ToolName: e.ToolName,
						Args:     e.Arguments,
					}
					// Collect tool calls; batch-execute by safety category after streaming completes.
					executor.Submit(toolCallInfo{
						toolID:    e.ToolID,
						toolName:  e.ToolName,
						arguments: e.Arguments,
					})
				case llm.StreamEnd:
					stopReason = e.StopReason
					usage = e.Usage
				}
			}
			a.emitHook(hooks.EventPostReceive, text.String(), nil)

			// Handle stream errors.
			select {
			case err := <-errs:
				if err != nil {
					if retry, compacted := a.handleStreamError(ctx, ch, conv, err); retry {
						if compacted {
							conv.ClearUsageAnchor()
							conv.InjectLongTermMemory(a.Instructions, a.MemoryContent, a.SkillSection)
						}
						continue // retry the turn
					}
					ch <- ErrorEvent{Message: err.Error()}
					return
				}
			default:
			}

			totalInput += usage.InputTokens
			totalOutput += usage.OutputTokens
			ch <- UsageEvent{InputTokens: totalInput, OutputTokens: totalOutput}

			anchorAfterAssistant := func() {
				conv.RecordUsageAnchor(
					usage.InputTokens,
					usage.OutputTokens,
					usage.CacheReadTokens,
					usage.CacheCreationTokens,
				)
			}

			// Handle max_tokens stop reason.
			if stopReason == "max_tokens" {
				if !maxTokensEscalated {
					// First hit: escalate silently.
					if setter, ok := a.Client.(llm.MaxTokensSetter); ok {
						setter.SetMaxOutputTokens(maxTokensCeiling)
						maxTokensEscalated = true
					}
					if text.String() != "" {
						conv.AddAssistantFull(text.String(), thinkingBlocks, nil)
						a.persistLastMessage(conv)
						anchorAfterAssistant()
						conv.AddUserMessage("Output token limit hit. Resume directly from where you stopped. Do not apologize or repeat previous content. Pick up mid-thought if needed.")
					}
					ch <- RetryEvent{Reason: "max_tokens escalation", Wait: 0}
					continue
				} else if outputRecoveries < maxOutputTokensRecoveries {
					// Multi-turn recovery.
					outputRecoveries++
					conv.AddAssistantFull(text.String(), thinkingBlocks, nil)
					a.persistLastMessage(conv)
					anchorAfterAssistant()
					conv.AddUserMessage("Output token limit hit. Resume directly from where you stopped. Break remaining work into smaller pieces.")
					ch <- RetryEvent{Reason: fmt.Sprintf("max_tokens recovery %d/%d", outputRecoveries, maxOutputTokensRecoveries), Wait: 0}
					continue
				}
				// Exhausted: fall through to normal completion.
			} else {
				// Reset recovery counter on successful turn.
				outputRecoveries = 0
			}

			if len(toolCalls) == 0 {
				conv.AddAssistantFull(text.String(), thinkingBlocks, nil)
				a.persistLastMessage(conv)
				if a.FileHistory != nil {
					summary := text.String()
					if len(summary) > 60 {
						summary = summary[:60] + "..."
					}
					a.FileHistory.MakeSnapshot(conv.Len(), summary)
				}
				ch <- LoopComplete{TotalTurns: iteration}
				if a.OnLoopComplete != nil {
					go a.OnLoopComplete(conv)
				}
				return
			}

			var toolUses []conversation.ToolUseBlock
			for _, tc := range toolCalls {
				toolUses = append(toolUses, conversation.ToolUseBlock{
					ToolUseID: tc.ToolID,
					ToolName:  tc.ToolName,
					Arguments: tc.Arguments,
				})
			}
			conv.AddAssistantFull(text.String(), thinkingBlocks, toolUses)
			a.persistLastMessage(conv)
			// Anchor real usage to the conversation now that the assistant message
			// is in place; subsequent tool results + next user message are
			// estimated incrementally on top of this baseline.
			anchorAfterAssistant()

			// Batch-execute by safety category: read-only tools concurrently, write/command tools serially.
			results := executor.ExecuteAll(ctx, a)

			// Spill-file readback results are exempt from spilling: if the content the model
			// just read back were persisted and replaced with a preview, the model would never
			// see the full text and would loop between "read back" and "spill" indefinitely.
			exempt := make(map[string]bool)
			for _, tc := range toolCalls {
				if tool_result.IsSpillReadback(tc.ToolName, tc.Arguments, a.WorkDir, a.SessionID) {
					exempt[tc.ToolID] = true
				}
			}

			var toolResults []conversation.ToolResultBlock
			for _, r := range results {
				ch <- ToolResultEvent{
					ToolID:   r.toolID,
					ToolName: r.toolName,
					Output:   r.output,
					IsError:  r.isError,
					Elapsed:  r.elapsed,
				}

				content := r.output
				if len(content) > tools.MaxOutputChars && !exempt[r.toolID] {
					// Single result exceeds limit: persist to disk and replace with preview.
					// If the write fails the original is retained; either way the result is
					// marked exempt so the aggregate budget won't retry it.
					content = tool_result.PersistLargeResult(a.WorkDir, a.SessionID, r.toolID, r.output)
					exempt[r.toolID] = true
				}
				toolResults = append(toolResults, conversation.ToolResultBlock{
					ToolUseID:     r.toolID,
					Content:       content,
					IsError:       r.isError,
					ContentBlocks: r.contentBlocks,
				})
			}

			// Aggregate budget: parallel tool results land in a single message, so the
			// per-item threshold cannot prevent the combined total from exceeding the limit.
			// Process the entire batch before it enters history so the message is born final.
			tool_result.ApplyBudget(toolResults, exempt, a.WorkDir, a.SessionID)

			exitPlanCalled := false
			for _, tc := range toolCalls {
				if tc.ToolName == "ExitPlanMode" {
					exitPlanCalled = true
					break
				}
			}
			conv.AddToolResultsMessage(toolResults)
			a.persistLastMessage(conv)

			// Non-blocking memory recall: check whether the prefetch is ready after tool execution.
			if a.MemoryRecallCh != nil {
				select {
				case recall := <-a.MemoryRecallCh:
					if recall.Reminder != "" {
						conv.AddSystemReminder(recall.Reminder)
						// Only mark as surfaced once actually injected. Unconsumed
						// recall results leave no trace so those memories remain
						// eligible in the next recall pass.
						a.MarkMemoriesSurfaced(recall.Paths)
					}
					a.MemoryRecallCh = nil // consume only once
				default:
					// Prefetch not ready yet; will check again next iteration.
				}
			}

			if exitPlanCalled {
				ch <- TurnComplete{Turn: iteration}
				ch <- LoopComplete{TotalTurns: iteration}
				return
			}
			ch <- TurnComplete{Turn: iteration}
			a.emitHook(hooks.EventTurnEnd, "", nil)
		}
	}()

	return ch
}

// emitHook fires a hook event when an Engine is configured. Failures are non-fatal and surface via
// the hook notification queue (drained into the next turn's system reminders).
func (a *Agent) emitHook(event hooks.EventName, message string, args map[string]any) {
	if a.Hooks == nil {
		return
	}
	a.Hooks.RunHooks(hooks.HookContext{
		EventName: event,
		ToolArgs:  args,
		Message:   message,
	})
}

// filterSchemasByName keeps only the tool schemas whose "name" passes the allow predicate. Used by
// Coordinator Mode to restrict a Lead agent to coordination-only tools while teammates do the
// actual work.
func filterSchemasByName(schemas []map[string]any, allow func(name string) bool) []map[string]any {
	out := make([]map[string]any, 0, len(schemas))
	for _, s := range schemas {
		name, _ := s["name"].(string)
		if allow(name) {
			out = append(out, s)
		}
	}
	return out
}

// handleStreamError returns (retry, compacted): retry signals the caller to
// re-run the turn; compacted signals that a ForceCompact rewrote the
// conversation, so the caller must drop its usage anchor (its AnchorCount no
// longer maps to the new transcript).
func (a *Agent) handleStreamError(ctx context.Context, ch chan AgentEvent, conv *conversation.Manager, err error) (retry, compacted bool) {
	var ctxErr *llm.ContextTooLongError
	if errors.As(err, &ctxErr) {
		// Tool results in history are already at their final form (budget applied on ingest),
		// so pass nil and let ForceCompact use conv's own messages directly.
		msg, compactErr := compact.ForceCompact(ctx, conv, a.Client, a.WorkDir, a.SessionID, a.ContextWindow, a.RecoveryState, a.currentToolSchemas())
		if compactErr == nil && msg != "" {
			ch <- CompactEvent{Message: "Auto-compacted due to context length: " + msg}
			return true, true // retry, and the anchor is now stale
		}
		return false, false
	}

	var rlErr *llm.RateLimitError
	if errors.As(err, &rlErr) {
		wait := parseRetryAfter(rlErr.RetryAfter)
		ch <- RetryEvent{Reason: "rate limited", Wait: wait}
		select {
		case <-time.After(wait):
			return true, false // retry without compaction
		case <-ctx.Done():
			return false, false
		}
	}

	return false, false
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 5 * time.Second
	}
	if secs, err := strconv.Atoi(header); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 5 * time.Second
}

type toolExecResult struct {
	toolID   string
	toolName string
	output   string
	isError  bool
	elapsed  time.Duration
	// contentBlocks lets tools pass structured content blocks through to the
	// conversation history. Only ToolSearch on the official endpoint populates
	// this (tool_reference); all other tools leave it empty and use plain text.
	contentBlocks []map[string]any
}

// extractFilePath pulls a representative path from common tool argument keys so hooks can do path-
// glob matching (`file_path =* "**/*.go"`).
func extractFilePath(args map[string]any) string {
	for _, key := range []string{"file_path", "path", "pattern", "target"} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func (a *Agent) executeSingleTool(ctx context.Context, eventCh chan AgentEvent, tc toolCallInfo) toolExecResult {
	tool := a.Registry.Get(tc.toolName)
	start := time.Now()

	// On invalid tool name, return a single error and let the model self-correct with another tool; keep the loop running.
	if tool == nil {
		return toolExecResult{
			toolID:   tc.toolID,
			toolName: tc.toolName,
			output:   fmt.Sprintf("Error: unknown tool '%s'", tc.toolName),
			isError:  true,
			elapsed:  time.Since(start),
		}
	}

	if a.Checker != nil {
		decision := a.Checker.Check(tool, tc.arguments)
		if decision.Effect == permissions.Deny {
			return toolExecResult{
				toolID:   tc.toolID,
				toolName: tc.toolName,
				output:   fmt.Sprintf("Permission denied: %s", decision.Reason),
				isError:  true,
				elapsed:  time.Since(start),
			}
		}
		if decision.Effect == permissions.Ask {
			respCh := make(chan PermissionResponse, 1)
			desc := permissions.DescribeToolAction(tc.toolName, tc.arguments)
			eventCh <- PermissionRequestEvent{
				ToolName:   tc.toolName,
				Desc:       desc,
				ResponseCh: respCh,
			}
			resp := <-respCh
			if resp == PermDeny {
				return toolExecResult{
					toolID:   tc.toolID,
					toolName: tc.toolName,
					output:   conversation.RejectedToolResult,
					isError:  true,
					elapsed:  time.Since(start),
				}
			}
			if resp == PermAllowAlways {
				content := permissions.ExtractContent(tc.toolName, tc.arguments)
				pattern := content + "*"
				if len(content) > 60 {
					pattern = content[:60] + "*"
				}
				// Write to the local rule file; the rule engine reads and matches
				// on every evaluation, so it takes effect right after this turn.
				a.Checker.RuleEngine.AppendLocalRule(permissions.Rule{
					ToolName: tc.toolName,
					Pattern:  pattern,
					Effect:   permissions.RuleAllow,
				})
			}
		}
	}

	if a.Hooks != nil {
		hookCtx := hooks.HookContext{
			EventName: hooks.EventPreToolUse,
			ToolName:  tc.toolName,
			ToolArgs:  tc.arguments,
			FilePath:  extractFilePath(tc.arguments),
		}
		if rejected, msg := a.Hooks.RunPreToolHooks(hookCtx); rejected {
			return toolExecResult{
				toolID:   tc.toolID,
				toolName: tc.toolName,
				output:   "Blocked by hook: " + msg,
				isError:  true,
				elapsed:  time.Since(start),
			}
		}
	}

	result := tool.Execute(ctx, tc.arguments)
	a.RecordRecentTool(tc.toolName)

	if !result.IsError && tc.toolName == "ReadFile" {
		if p, _ := tc.arguments["file_path"].(string); p != "" {
			if data, err := os.ReadFile(p); err == nil {
				a.RecoveryState.RecordFileRead(p, string(data))
			}
		}
	}

	if a.Hooks != nil {
		a.Hooks.RunHooks(hooks.HookContext{
			EventName: hooks.EventPostToolUse,
			ToolName:  tc.toolName,
			ToolArgs:  tc.arguments,
			FilePath:  extractFilePath(tc.arguments),
			Message:   result.Output,
		})
	}

	return toolExecResult{
		toolID:        tc.toolID,
		toolName:      tc.toolName,
		output:        result.Output,
		isError:       result.IsError,
		elapsed:       time.Since(start),
		contentBlocks: result.ContentBlocks,
	}
}

func formatToolArgs(args map[string]any) string {
	var parts []string
	for k, v := range args {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "…"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", k, s))
	}
	return strings.Join(parts, ", ")
}

// persistLastMessage writes the most recently appended conversation message to the session log.
//
// Persistence lives in the main loop rather than in individual frontends: both TUI and Web share
// the same recording path, ensuring intermediate assistant text and complete tool-call chains are
// captured so sessions can be faithfully restored on resume. Skipped (no disk write) when WorkDir
// or SessionID is empty (one-shot callers, sub-agents).
func (a *Agent) persistLastMessage(conv *conversation.Manager) {
	if a.WorkDir == "" || a.SessionID == "" {
		return
	}
	msgs := conv.GetMessages()
	if len(msgs) == 0 {
		return
	}
	session.SaveMessage(a.WorkDir, a.SessionID, session.FromConversation(msgs[len(msgs)-1]))
}
