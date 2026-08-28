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

// Package compact implements Swiftx's Layer 2 context management:
// an LLM-driven full-conversation summary, gated by token ratio (default
// >80% of context window). Replaces the entire conversation with a summary
// message + a continuation acknowledgement. Also reachable via ForceCompact
// (the /compact slash command).
//
// Layer 1 (tool-result budget) lives in package toolresult: both single-result
// overruns and per-message aggregate overruns are handled at the moment a
// result enters the conversation history. Once a message is in history it is
// in its final form, so the message sizes observed here are the final sizes.

package compact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
)

const (
	// maxPTLRetries limits how many times we retry the summary request after
	// a prompt-too-long error by dropping the oldest API-round groups.
	maxPTLRetries = 3
	// ptlRetryMarker is prepended after dropping old groups so the summary
	// request still starts with a user-role message.
	ptlRetryMarker = "[earlier conversation truncated for compaction retry]"

	// autoCompactThreshold is the legacy ratio gate (kept for reference only).
	// The live decision now uses the absolute-token formula below:
	// trigger compaction when used tokens approach the context window limit, leaving a margin for the next turn.
	autoCompactThreshold = 0.80

	// summaryOutputReserve reserves room for the summary response itself, so the
	// effective window is contextWindow − min(model maxOutput, summaryOutputReserve).
	summaryOutputReserve = 20000
	// autoCompactSafetyMargin sets the soft auto-compact trigger line below the
	// effective window.
	autoCompactSafetyMargin = 13000
	// manualCompactSafetyMargin sets the hard-block line: once used tokens cross
	// effectiveWindow − manualCompactSafetyMargin, we force a compaction rather
	// than rely on the soft trigger.
	manualCompactSafetyMargin = 3000
)

// Recent-message retention budget for compaction: instead of summarizing the
// whole transcript and discarding every original message, we keep the tail of
// recent messages verbatim and only summarize the older prefix.
const (
	// keepRecentTokens is the lower-bound token budget: walk back from the tail
	// accumulating per-message tokens until we've kept at least this many.
	keepRecentTokens = 10000
	// minKeepMessages is the minimum number of recent messages to keep regardless
	// of token count. Either keepRecentTokens or minKeepMessages satisfied is
	// enough to stop walking back (whichever comes first).
	minKeepMessages = 5
	// keepMaxTokens caps the kept tail: once accumulated tokens would exceed this,
	// stop walking back even if the lower bounds aren't met, so we never keep so
	// much that the summary saves nothing.
	keepMaxTokens = 40000
)

// computeCompactThreshold returns the absolute used-token line at which Layer 2
// should fire. effectiveWindow = contextWindow − min(maxOutput, summaryOutputReserve);
// the threshold is effectiveWindow minus the safety margin (manual margin for the
// hard-block line, auto margin for the soft trigger).
func computeCompactThreshold(contextWindow, maxOutput int, manual bool) int {
	reserve := summaryOutputReserve
	if maxOutput > 0 && maxOutput < reserve {
		reserve = maxOutput
	}
	effectiveWindow := contextWindow - reserve
	margin := autoCompactSafetyMargin
	if manual {
		margin = manualCompactSafetyMargin
	}
	return effectiveWindow - margin
}

// MaxConsecutiveAutoCompactFailures stops auto-compact retries when the context is irrecoverably
// over the limit (e.g., prompt_too_long), so the agent doesn't hammer the API with doomed attempts
// on every iteration.
const MaxConsecutiveAutoCompactFailures = 3

// AutoCompactTrackingState threads circuit-breaker state across agent loop iterations. The caller
// owns the struct; ManageContext mutates it in place.
type AutoCompactTrackingState struct {
	// ConsecutiveFailures counts auto-compact attempts that returned an error since the last success.
	// Reset to zero on success.
	ConsecutiveFailures int
}

// summarySystemPrompt instructs the model to produce a two-phase response: a <analysis> scratchpad
// block followed by a <summary> block. The analysis block is stripped by formatCompactSummary
// before the result is written back into the conversation, leaving only the structured summary.
const summarySystemPrompt = `Your task is to create a detailed summary of the conversation so far, paying close attention to the user's explicit requests and your previous actions.
This summary should be thorough in capturing technical details, code patterns, and architectural decisions that would be essential for continuing development work without losing context.

Before providing your final summary, wrap your analysis in <analysis> tags to organize your thoughts and ensure you've covered all necessary points. In your analysis process:

1. Chronologically analyze each message and section of the conversation. For each section thoroughly identify:
   - The user's explicit requests and intents
   - Your approach to addressing the user's requests
   - Key decisions, technical concepts and code patterns
   - Specific details like:
     - file names
     - full code snippets
     - function signatures
     - file edits
   - Errors that you ran into and how you fixed them
   - Pay special attention to specific user feedback that you received, especially if the user told you to do something differently.
2. Double-check for technical accuracy and completeness, addressing each required element thoroughly.

After your analysis, output your final summary wrapped in <summary> tags. Your summary should include the following sections:

1. Primary Request and Intent: Capture all of the user's explicit requests and intents in detail
2. Key Technical Concepts: List all important technical concepts, technologies, and frameworks discussed.
3. Files and Code Sections: Enumerate specific files and code sections examined, modified, or created. Pay special attention to the most recent messages and include full code snippets where applicable and include a summary of why this file read or edit is important.
4. Errors and fixes: List all errors that you ran into, and how you fixed them. Pay special attention to specific user feedback that you received, especially if the user told you to do something differently.
5. Problem Solving: Document problems solved and any ongoing troubleshooting efforts.
6. All user messages: List ALL user messages that are not tool results. These are critical for understanding the users' feedback and changing intent.
7. Pending Tasks: Outline any pending tasks that you have explicitly been asked to work on.
8. Current Work: Describe in detail precisely what was being worked on immediately before this summary request, paying special attention to the most recent messages from both user and assistant. Include file names and code snippets where applicable.
9. Optional Next Step: List the next step that you will take that is related to the most recent work you were doing. IMPORTANT: ensure that this step is DIRECTLY in line with the user's most recent explicit requests, and the task you were working on immediately before this summary request. If your last task was concluded, then only list next steps if they are explicitly in line with the users request.
   If there is a next step, include direct quotes from the most recent conversation showing exactly what task you were working on and where you left off. This should be verbatim to ensure there's no drift in task interpretation.

Output structure:

<analysis>
[Your thought process, ensuring all points are covered thoroughly and accurately]
</analysis>

<summary>
1. Primary Request and Intent:
   [Detailed description]

2. Key Technical Concepts:
   - [Concept 1]
   - [Concept 2]

3. Files and Code Sections:
   - [File Name 1]
      - [Summary and important code snippet]

4. Errors and fixes:
   - [Error and fix description]

5. Problem Solving:
   [Description]

6. All user messages:
   - [User message 1]
   - [User message 2]

7. Pending Tasks:
   - [Task 1]

8. Current Work:
   [Precise description]

9. Optional Next Step:
   [Next step if applicable]
</summary>`

// EstimateTokens uses a 3.5 chars/token approximation across content, tool args, tool results, and
// thinking blocks.
func EstimateTokens(messages []conversation.Message) int {
	total := 0
	for _, m := range messages {
		total += int(float64(len(m.Content))/3.5) + 4
		for _, tu := range m.ToolUses {
			argsJSON, _ := json.Marshal(tu.Arguments)
			total += 50 + int(float64(len(argsJSON))/3.5)
		}
		for _, tr := range m.ToolResults {
			total += int(float64(len(tr.Content))/3.5) + 10
		}
		for _, tb := range m.ThinkingBlocks {
			total += int(float64(len(tb.Thinking)) / 3.5)
		}
	}
	return total
}

// UsageAnchor records the last real API usage and the conversation length at the
// moment that usage was reported. baselineTokens is the total true prompt+output
// size for that turn (input + cache_read + cache_creation + output); anchorCount
// is conv.Len() right after the assistant turn settled. Anything appended past
// anchorCount (tool results, the next user message, system reminders) has no real
// usage yet, so it's estimated incrementally on top of the baseline.
//
// A zero-value anchor (HasUsage false) means no real usage has been observed yet
// — the first turn — so callers fall back to a full character estimate.
type UsageAnchor struct {
	BaselineTokens int
	AnchorCount    int
	HasUsage       bool
}

// BaselineFromUsage collapses an API usage report into the single "true tokens
// already on the wire" number used as the anchor baseline. Anthropic reports
// cache_read / cache_creation separately from input_tokens, so the real prompt
// size is the sum of all four counters.
func BaselineFromUsage(u llm.UsageInfo) int {
	return u.InputTokens + u.CacheReadTokens + u.CacheCreationTokens + u.OutputTokens
}

// ComputeUsedTokens returns the current "used tokens" figure that the compaction
// threshold is compared against. When a real-usage anchor exists, it returns
// baselineTokens + an estimate of only the messages appended after the anchor
// (incremental estimate). Without an anchor (cold start, first turn) it falls
// back to a full character estimate over every message, matching the original
// behaviour so the agent stays usable before the first usage report lands.
func ComputeUsedTokens(messages []conversation.Message, anchor UsageAnchor) int {
	if !anchor.HasUsage {
		return EstimateTokens(messages)
	}
	// Defensive clamp: if the conversation was rewound below the anchor (e.g.
	// after a compaction) the anchor is stale, so estimate everything instead
	// of indexing out of range.
	if anchor.AnchorCount < 0 || anchor.AnchorCount > len(messages) {
		return EstimateTokens(messages)
	}
	return anchor.BaselineTokens + EstimateTokens(messages[anchor.AnchorCount:])
}

// ComputeUsedTokensFromConv reads the anchor state from the ConversationManager
// to compute the current token usage. It is a convenience wrapper around
// ComputeUsedTokens that spares callers from passing the UsageAnchor manually.
func ComputeUsedTokensFromConv(conv *conversation.Manager) int {
	baseline, count, has := conv.UsageAnchorState()
	return ComputeUsedTokens(conv.GetMessages(), UsageAnchor{
		BaselineTokens: baseline,
		AnchorCount:    count,
		HasUsage:       has,
	})
}

// ManageContext runs Layer 2 (autoCompact) when used tokens reach the
// auto-compact threshold (effectiveWindow − auto margin); once they cross the
// hard-block line (effectiveWindow − manual margin) it forces a compaction.
// See computeCompactThreshold. Layer 1 (tool-result budget) runs at ingest —
// tool results are already final-size by the time they enter history, so the
// estimates here need no further trimming.
//
// tracking carries the circuit-breaker state across iterations. When nil,
// the circuit breaker is disabled (useful for tests and one-shot callers).
//
// anchor carries the most recent real API usage (baseline + conversation length
// at the time it was reported). When present, the used-token figure is
// baselineTokens + an incremental estimate of only the messages appended since;
// when absent (first turn) it falls back to a full character estimate. This
// only changes how "currently used tokens" is computed — the threshold formula
// (effectiveWindow − margin) is untouched.
//
// `workDir` + `sessionID` locate the on-disk session log; when both are
// non-empty, a successful compaction appends a compact_boundary record there so
// a later resume can rebuild the compacted state instead of replaying the full
// pre-compaction transcript. When either is empty, boundary persistence is
// skipped (tests, one-shot callers) and behaviour is unchanged.
func ManageContext(
	ctx context.Context,
	conv *conversation.Manager,
	client llm.Client,
	workDir string,
	sessionID string,
	contextWindow int,
	maxOutput int,
	tracking *AutoCompactTrackingState,
	recovery *RecoveryState,
	toolSchemas []map[string]any,
) (string, error) {
	// Tool results in history were already budgeted to their final size at
	// ingest time, so conv's own messages reflect the actual payload sent;
	// estimate directly from them.
	baseline, count, has := conv.UsageAnchorState()
	anchor := UsageAnchor{BaselineTokens: baseline, AnchorCount: count, HasUsage: has}
	tokens := ComputeUsedTokens(conv.GetMessages(), anchor)
	// Soft auto-compact trigger: used tokens >= effectiveWindow − auto margin.
	if tokens < computeCompactThreshold(contextWindow, maxOutput, false) {
		return "", nil
	}

	// Hard-block line: once used tokens cross effectiveWindow − manual margin,
	// force a compaction (bypassing the circuit breaker) instead of the soft
	// auto path, since the context is too close to the wall to risk skipping.
	if tokens >= computeCompactThreshold(contextWindow, maxOutput, true) {
		return ForceCompact(ctx, conv, client, workDir, sessionID, contextWindow, recovery, toolSchemas)
	}

	// Circuit breaker: stop retrying after N consecutive failures. Without
	// this, sessions where context is irrecoverably over the limit hammer
	// the API with doomed compaction attempts on every iteration.
	if tracking != nil && tracking.ConsecutiveFailures >= MaxConsecutiveAutoCompactFailures {
		return "", nil
	}

	msg, err := autoCompact(ctx, conv, client, workDir, sessionID, contextWindow, recovery, toolSchemas)
	if err != nil {
		if tracking != nil {
			tracking.ConsecutiveFailures++
		}
		return "", err
	}
	if tracking != nil {
		tracking.ConsecutiveFailures = 0
	}
	return msg, nil
}

// ForceCompact is the manual /compact entry. Always runs Layer 2 (full summary) regardless of
// current token ratio. Layer 1 is skipped because a full summary supersedes the
// tool-result budget anyway.
func ForceCompact(
	ctx context.Context,
	conv *conversation.Manager,
	client llm.Client,
	workDir string,
	sessionID string,
	contextWindow int,
	recovery *RecoveryState,
	toolSchemas []map[string]any,
) (string, error) {
	return autoCompact(ctx, conv, client, workDir, sessionID, contextWindow, recovery, toolSchemas)
}

// hasToolResults reports whether a message carries any tool_result blocks (a
// user-role message produced by the tool executor). Such a message must not be
// separated from the assistant tool_use it answers, or the API rejects the
// conversation as having an orphaned tool_result.
func hasToolResults(m conversation.Message) bool {
	return len(m.ToolResults) > 0
}

// hasToolUses reports whether a message carries any tool_use blocks (an
// assistant message that called a tool).
func hasToolUses(m conversation.Message) bool {
	return len(m.ToolUses) > 0
}

// computeKeepStartIndex chooses the boundary between the prefix that gets
// summarized (messages[:keepStart]) and the recent tail kept verbatim
// (messages[keepStart:]).
//
// Walk back from the tail accumulating per-message tokens (single-message
// EstimateTokens). Stop once EITHER the token lower bound (keepRecentTokens) OR
// the message-count lower bound (minKeepMessages) is met — whichever comes
// first is enough. But never let the accumulated tail exceed keepMaxTokens: if
// including one more message would cross that cap, stop before it.
//
// After the budget walk, snap the boundary backward so it never splits a
// tool_use ↔ tool_result pair: if keepStart lands on a message bearing
// tool_results, move it back past the preceding assistant tool_use message(s)
// so the pair stays whole in the kept tail (keep a full pair rather than an
// orphaned tool_result).
//
// Returns the index; callers treat keepStart <= 0 (or a tiny prefix) as "too
// little to compact" and fall back to the original full-summary behaviour.
func computeKeepStartIndex(messages []conversation.Message) int {
	n := len(messages)
	if n == 0 {
		return 0
	}

	keptTokens := 0
	keptCount := 0
	keepStart := n
	for i := n - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i : i+1])
		// Upper bound: if adding this message would exceed the cap, stop and
		// leave it in the summarized prefix (don't keep it).
		if keptCount > 0 && keptTokens+msgTokens > keepMaxTokens {
			break
		}
		keptTokens += msgTokens
		keptCount++
		keepStart = i
		// Lower bounds: either satisfied → done.
		if keptTokens >= keepRecentTokens || keptCount >= minKeepMessages {
			break
		}
	}

	// Don't split a tool_use ↔ tool_result pair: if the boundary message holds
	// tool_results, move it back across the assistant tool_use message(s) it
	// answers so the pair is kept intact.
	for keepStart > 0 && hasToolResults(messages[keepStart]) {
		prev := keepStart - 1
		if hasToolUses(messages[prev]) {
			keepStart = prev
			continue
		}
		break
	}

	return keepStart
}

// groupMessagesByAPIRound splits messages into groups at API round-trip
// boundaries. A new group starts at each assistant message that follows a
// tool_result (i.e. a new LLM turn). This keeps tool_use/tool_result pairs
// together so dropping a group never orphans a tool_result.
func groupMessagesByAPIRound(messages []conversation.Message) [][]conversation.Message {
	var groups [][]conversation.Message
	var current []conversation.Message
	prevHadToolResult := false

	for _, m := range messages {
		if m.Role == "assistant" && prevHadToolResult && len(current) > 0 {
			groups = append(groups, current)
			current = nil
		}
		current = append(current, m)
		prevHadToolResult = len(m.ToolResults) > 0
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// truncateHeadForPTL drops the oldest API-round groups from the prefix
// messages until the estimated token count drops by at least tokenGap.
// Returns nil if there's nothing meaningful left to summarize.
func truncateHeadForPTL(prefix []conversation.Message, tokenGap int) []conversation.Message {
	groups := groupMessagesByAPIRound(prefix)
	if len(groups) < 2 {
		return nil
	}

	dropCount := 0
	if tokenGap > 0 {
		acc := 0
		for _, g := range groups {
			acc += EstimateTokens(g)
			dropCount++
			if acc >= tokenGap {
				break
			}
		}
	} else {
		dropCount = max(1, len(groups)/5)
	}

	dropCount = min(dropCount, len(groups)-1)
	if dropCount < 1 {
		return nil
	}

	var result []conversation.Message
	for _, g := range groups[dropCount:] {
		result = append(result, g...)
	}
	if len(result) > 0 && result[0].Role != "user" {
		marker := conversation.Message{Role: "user", Content: ptlRetryMarker}
		result = append([]conversation.Message{marker}, result...)
	}
	return result
}

// buildPrefixText serializes prefix messages into a text block for the
// summary LLM call.
func buildPrefixText(prefix []conversation.Message) string {
	var sb strings.Builder
	for _, m := range prefix {
		fmt.Fprintf(&sb, "[%s]: %s\n", m.Role, m.Content)
		for _, tu := range m.ToolUses {
			fmt.Fprintf(&sb, "[tool_use %s]: %s\n", tu.ToolName, tu.ToolUseID)
		}
		for _, tr := range m.ToolResults {
			content := tr.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			fmt.Fprintf(&sb, "[tool_result]: %s\n", content)
		}
	}
	return sb.String()
}

// autoCompact is Layer 2: an LLM summary of the older prefix that replaces only
// messages[:keepStart] with a single summary message, while the recent tail
// (messages[keepStart:]) is preserved verbatim. After the summary lands, a recovery
// block is appended to the summary message so the model still has snapshots of
// the files it just read, the SOPs for any skills it invoked, and the current
// tool listing. When there's too little prefix to summarize, it degrades to the
// original full-summary behaviour.
func autoCompact(
	ctx context.Context,
	conv *conversation.Manager,
	client llm.Client,
	workDir string,
	sessionID string,
	contextWindow int,
	recovery *RecoveryState,
	toolSchemas []map[string]any,
) (string, error) {
	messages := conv.GetMessages()
	beforeTokens := EstimateTokens(messages)

	// Select the recent tail to keep verbatim; only the prefix before keepStart
	// is summarized. If the boundary leaves nothing (or almost nothing) to
	// summarize, there's no point compacting — keep the conversation untouched.
	keepStart := computeKeepStartIndex(messages)
	if keepStart <= 0 {
		// Everything is within the kept tail (conversation too short) — degrade
		// to no-op so we don't "compact for nothing".
		return "", nil
	}
	prefix := messages[:keepStart]
	keep := messages[keepStart:]

	// Invoke the LLM to produce the summary, with PTL retry: if the summary
	// request itself exceeds the context window, drop the oldest API rounds
	// and retry, up to maxPTLRetries times.
	finalSummary, err := callSummaryWithPTLRetry(ctx, client, prefix, toolSchemas)
	if err != nil {
		return "", err
	}

	// Persist a compact_boundary record so a later resume can rebuild this
	// compacted state (summary + kept tail) instead of replaying the full
	// pre-compaction transcript. Append-only: the original prefix messages stay
	// in the session file but won't be replayed past this boundary. We inline the
	// kept tail with its tool blocks, so a resumed session keeps the full
	// tool-call chain of those recent turns. The boundary stores the pure summary
	// text, not the recovery attachment, since the recovery snapshots are an
	// in-memory rebuild aid unavailable on resume. Skipped when sessionID/workDir
	// is empty (tests, one-shot callers).
	if sessionID != "" && workDir != "" {
		keepRecords := make([]session.KeepMessage, 0, len(keep))
		for _, m := range keep {
			keepRecords = append(keepRecords, session.FromConversationKeep(m))
		}
		session.SaveCompactBoundary(workDir, sessionID, finalSummary, keepRecords)
	}

	content := "This session continues a previous conversation that was compacted because the context window was running low. Below is a summary of the earlier conversation:\n\n" + finalSummary
	if len(keep) > 0 {
		content += "\n\nRecent messages have been preserved verbatim."
	}
	if sessionID != "" && workDir != "" {
		content += fmt.Sprintf("\n\nIf you need specifics from before the compaction (code snippets, error messages, etc.), use ReadFile to load the full session log: %s", session.SessionFilePath(workDir, sessionID))
	}
	if attachment := BuildRecoveryAttachment(recovery, toolSchemas); attachment != "" {
		content += "\n\n---\n\n" + attachment
	}

	compacted := conversation.NewManager()
	compacted.AddUserMessage(content)
	compacted.AppendMessages(keep)

	*conv = *compacted
	afterTokens := EstimateTokens(conv.GetMessages())
	return fmt.Sprintf("Compacted: %d → %d estimated tokens", beforeTokens, afterTokens), nil
}

// callSummaryWithPTLRetry sends the prefix to the LLM for summarization. If
// the request returns ContextTooLongError, it drops the oldest API-round groups
// from the prefix and retries, up to maxPTLRetries times.
func callSummaryWithPTLRetry(
	ctx context.Context,
	client llm.Client,
	prefix []conversation.Message,
	toolSchemas []map[string]any,
) (string, error) {
	currentPrefix := prefix
	for attempt := 0; ; attempt++ {
		text := buildPrefixText(currentPrefix)
		summaryConv := conversation.NewManager()
		summaryConv.AddUserMessage(summarySystemPrompt + "\n\n" + text)

		events, errs := client.Stream(ctx, summaryConv, toolSchemas)
		var summary strings.Builder
		for ev := range events {
			if td, ok := ev.(llm.TextDelta); ok {
				summary.WriteString(td.Text)
			}
		}
		var streamErr error
		select {
		case streamErr = <-errs:
		default:
		}

		if streamErr == nil {
			return formatCompactSummary(summary.String()), nil
		}

		var ptlErr *llm.ContextTooLongError
		if !errors.As(streamErr, &ptlErr) || attempt >= maxPTLRetries {
			return "", streamErr
		}

		tokenGap := EstimateTokens(currentPrefix) / 5
		truncated := truncateHeadForPTL(currentPrefix, tokenGap)
		if truncated == nil {
			return "", streamErr
		}
		currentPrefix = truncated
	}
}

// formatCompactSummary strips the <analysis> scratchpad block from the model's two-phase response,
// returning only the contents of the <summary> block. Falls back to the raw text when neither tag
// is present (model disobeyed the prompt structure) so we never lose the summary altogether.
func formatCompactSummary(raw string) string {
	if _, after, ok := strings.Cut(raw, "<summary>"); ok {
		body := after
		if before, _, ok := strings.Cut(body, "</summary>"); ok {
			return strings.TrimSpace(before)
		}
		return strings.TrimSpace(body)
	}
	// No <summary> block — drop any <analysis>.</analysis> block if present and return what's left.
	if start := strings.Index(raw, "<analysis>"); start >= 0 {
		if end := strings.Index(raw, "</analysis>"); end > start {
			return strings.TrimSpace(raw[:start] + raw[end+len("</analysis>"):])
		}
	}
	return strings.TrimSpace(raw)
}
