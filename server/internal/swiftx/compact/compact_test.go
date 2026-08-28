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

package compact

import (
	"context"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/session"
)

// stubSummaryClient implements llm.Client and streams a fixed <summary> block,
// so autoCompact's summarization step is deterministic in tests. It records the
// prompt it was asked to summarize so tests can assert that only the prefix —
// not the kept tail — was summarized.
type stubSummaryClient struct {
	summary      string
	lastPrompt   string
	streamCalled bool
}

func (c *stubSummaryClient) SetSystemPrompt(prompt string) {}

func (c *stubSummaryClient) Stream(ctx context.Context, conv *conversation.Manager, tools []map[string]any) (<-chan llm.StreamEvent, <-chan error) {
	c.streamCalled = true
	if msgs := conv.GetMessages(); len(msgs) > 0 {
		c.lastPrompt = msgs[len(msgs)-1].Content
	}
	ch := make(chan llm.StreamEvent, 4)
	errCh := make(chan error, 1)
	ch <- llm.TextDelta{Text: "<summary>" + c.summary + "</summary>"}
	ch <- llm.StreamEnd{StopReason: "end_turn"}
	close(ch)
	errCh <- nil
	close(errCh)
	return ch, errCh
}

// Layer 1 (offload + snip) tests have moved to internal/toolresult/budget_test.go
// where the implementation now lives. compact only owns Layer 2 (autoCompact)
// plus the formatCompactSummary helper, so this file covers those.

func TestFormatCompactSummary(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "both blocks present",
			in:   "<analysis>scratch thoughts</analysis>\n<summary>final text</summary>",
			want: "final text",
		},
		{
			name: "summary block unterminated",
			in:   "<analysis>scratch</analysis>\n<summary>tail with no close tag",
			want: "tail with no close tag",
		},
		{
			name: "only analysis block — drop it",
			in:   "prefix <analysis>scratch</analysis> suffix",
			want: "prefix  suffix",
		},
		{
			name: "neither block — return raw",
			in:   "plain text response",
			want: "plain text response",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatCompactSummary(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// EstimateTokens covers all content sources without crashing on empty.
func TestEstimateTokensZeroAndPopulated(t *testing.T) {
	if got := EstimateTokens(nil); got != 0 {
		t.Errorf("empty input should be 0 tokens, got %d", got)
	}
	conv := conversation.NewManager()
	conv.AddUserMessage(strings.Repeat("x", 700))
	got := EstimateTokens(conv.GetMessages())
	if got < 150 || got > 250 {
		t.Errorf("700-char message should estimate ~200 tokens, got %d", got)
	}
}

// BaselineFromUsage must sum all four real-token counters so the anchor reflects
// the true prompt+output size even when cache hits dominate (input small,
// cache_read large).
func TestBaselineFromUsage(t *testing.T) {
	u := llm.UsageInfo{
		InputTokens:         100,
		OutputTokens:        40,
		CacheReadTokens:     5000,
		CacheCreationTokens: 200,
	}
	if got, want := BaselineFromUsage(u), 5340; got != want {
		t.Errorf("BaselineFromUsage = %d, want %d", got, want)
	}
	// Zero usage (compat endpoint that reports nothing) → zero baseline so the
	// caller knows not to adopt it as an anchor.
	if got := BaselineFromUsage(llm.UsageInfo{}); got != 0 {
		t.Errorf("empty usage baseline = %d, want 0", got)
	}
}

// ComputeUsedTokens: with no anchor (cold start / first turn) it must fall back
// to a full character estimate over every message, matching EstimateTokens.
func TestComputeUsedTokensColdStartFallback(t *testing.T) {
	conv := conversation.NewManager()
	conv.AddUserMessage(strings.Repeat("x", 700))
	conv.AddAssistantMessage(strings.Repeat("y", 700))
	msgs := conv.GetMessages()

	got := ComputeUsedTokens(msgs, UsageAnchor{}) // HasUsage == false
	want := EstimateTokens(msgs)
	if got != want {
		t.Errorf("cold-start ComputeUsedTokens = %d, want full estimate %d", got, want)
	}
}

// ComputeUsedTokens: with an anchor it must return baseline + an estimate of
// ONLY the messages appended past anchorCount — not a re-estimate of the whole
// transcript. This is the cache-hit win: the real input was far smaller than the
// character count of the anchored prefix.
func TestComputeUsedTokensWithAnchorIncremental(t *testing.T) {
	conv := conversation.NewManager()
	// 3 large anchored messages: their real token cost is captured by baseline,
	// NOT by their character count.
	conv.AddUserMessage(strings.Repeat("x", 7000))
	conv.AddAssistantMessage(strings.Repeat("y", 7000))
	conv.AddUserMessage(strings.Repeat("z", 7000))
	anchorCount := conv.Len()
	// One small message appended after the anchor.
	conv.AddAssistantMessage(strings.Repeat("w", 350))
	msgs := conv.GetMessages()

	const baseline = 1500 // pretend the real API said the prefix cost 1500 tokens
	anchor := UsageAnchor{BaselineTokens: baseline, AnchorCount: anchorCount, HasUsage: true}

	got := ComputeUsedTokens(msgs, anchor)
	wantIncrement := EstimateTokens(msgs[anchorCount:])
	if got != baseline+wantIncrement {
		t.Errorf("anchored ComputeUsedTokens = %d, want baseline+increment %d", got, baseline+wantIncrement)
	}
	// Sanity: the incremental result must be far below a full character estimate
	// of the (cache-heavy) transcript, proving we didn't re-estimate the prefix.
	if full := EstimateTokens(msgs); got >= full {
		t.Errorf("anchored result %d should be below full estimate %d", got, full)
	}
}

// ComputeUsedTokens: a stale anchor (AnchorCount past the current message count,
// e.g. after a compaction rewound the transcript) must not panic and should fall
// back to a full estimate.
func TestComputeUsedTokensStaleAnchorClamp(t *testing.T) {
	conv := conversation.NewManager()
	conv.AddUserMessage("hi")
	msgs := conv.GetMessages()

	anchor := UsageAnchor{BaselineTokens: 9999, AnchorCount: 50, HasUsage: true}
	got := ComputeUsedTokens(msgs, anchor)
	if want := EstimateTokens(msgs); got != want {
		t.Errorf("stale-anchor ComputeUsedTokens = %d, want full estimate %d", got, want)
	}
}

// bigMsg returns a message whose content alone estimates to roughly `tokens`
// tokens (recoveryCharsPerToken ≈ 3.5 chars/token), so tests can drive the
// keepRecentTokens budget walk deterministically.
func bigMsg(tokens int) string {
	return strings.Repeat("x", tokens*4)
}

// containsMsg reports whether any message in msgs has content equal to want.
func containsMsg(msgs []conversation.Message, want string) bool {
	for _, m := range msgs {
		if m.Content == want {
			return true
		}
	}
	return false
}

// autoCompact must keep the recent tail verbatim, not replace it with the
// summary. We build a transcript whose older prefix is large enough to clear
// keepStart > 0 and whose tail carries distinctive content; after compaction
// the tail content must still be present (not only the summary).
func TestAutoCompactKeepsRecentVerbatim(t *testing.T) {
	conv := conversation.NewManager()
	// Older prefix: several large messages that should be summarized away.
	for range 6 {
		conv.AddUserMessage("OLD-PREFIX " + bigMsg(3000))
		conv.AddAssistantMessage("OLD-REPLY " + bigMsg(3000))
	}
	// Recent tail: distinctive small messages we expect to survive verbatim.
	recent := []string{"RECENT-A unique-marker-A", "RECENT-B unique-marker-B"}
	conv.AddUserMessage(recent[0])
	conv.AddAssistantMessage(recent[1])

	client := &stubSummaryClient{summary: "THE SUMMARY"}
	msg, err := autoCompact(context.Background(), conv, client, "", "", 200000, nil, nil)
	if err != nil {
		t.Fatalf("autoCompact error: %v", err)
	}
	if msg == "" {
		t.Fatalf("expected a compaction message, got empty (degraded to no-op)")
	}
	out := conv.GetMessages()

	// Summary must be present.
	var sawSummary bool
	for _, m := range out {
		if strings.Contains(m.Content, "THE SUMMARY") {
			sawSummary = true
		}
	}
	if !sawSummary {
		t.Errorf("summary not present after compaction")
	}
	// Recent tail must be preserved verbatim, not collapsed into the summary.
	for _, r := range recent {
		if !containsMsg(out, r) {
			t.Errorf("recent message %q not preserved verbatim after compaction; messages=%v", r, msgContents(out))
		}
	}
}

// When a sessionID + workDir are wired, autoCompact must persist a
// compact_boundary record into the session log: the inlined summary plus the
// kept tail (role+content). This is the on-disk half of the resume round-trip —
// session.FindLastCompactBoundary then rebuilds the compacted state from it.
func TestAutoCompactPersistsBoundary(t *testing.T) {
	conv := conversation.NewManager()
	for range 6 {
		conv.AddUserMessage("OLD-PREFIX " + bigMsg(3000))
		conv.AddAssistantMessage("OLD-REPLY " + bigMsg(3000))
	}
	conv.AddUserMessage("RECENT-TAIL-USER unique-marker-A")
	conv.AddAssistantMessage("RECENT-TAIL-ASSISTANT unique-marker-B")

	workDir := t.TempDir()
	sid := "compact-roundtrip"
	client := &stubSummaryClient{summary: "PERSISTED-SUMMARY"}

	msg, err := autoCompact(context.Background(), conv, client, workDir, sid, 200000, nil, nil)
	if err != nil {
		t.Fatalf("autoCompact error: %v", err)
	}
	if msg == "" {
		t.Fatalf("expected a compaction message, got empty (degraded to no-op)")
	}

	// Read the session log back and assert the boundary was written with the
	// summary + the kept tail inlined.
	msgs := session.LoadSession(workDir, sid)
	boundary, after, ok := session.FindLastCompactBoundary(msgs)
	if !ok {
		t.Fatalf("expected a compact_boundary record to be persisted")
	}
	if boundary.Summary != "PERSISTED-SUMMARY" {
		t.Fatalf("persisted summary mismatch: got %q", boundary.Summary)
	}
	if len(after) != 0 {
		t.Fatalf("no messages should follow a freshly written boundary, got %d", len(after))
	}
	// The kept tail must be inlined verbatim into the boundary.
	var sawTailUser, sawTailAssistant bool
	for _, k := range boundary.Keep {
		if k.Content == "RECENT-TAIL-USER unique-marker-A" {
			sawTailUser = true
		}
		if k.Content == "RECENT-TAIL-ASSISTANT unique-marker-B" {
			sawTailAssistant = true
		}
	}
	if !sawTailUser || !sawTailAssistant {
		t.Fatalf("kept tail not inlined into boundary: %+v", boundary.Keep)
	}
	// The boundary's kept tail must exactly equal the conversation's tail that
	// autoCompact preserved verbatim (same role+content, in order). The summary
	// stored on disk is the pure summary text, and after the in-memory rebuild
	// the conversation is [summary user msg] + [continuation ack] + keep, so the
	// kept tail lives at the end of the rebuilt conversation.
	rebuilt := conv.GetMessages()
	tail := rebuilt[len(rebuilt)-len(boundary.Keep):]
	for i, k := range boundary.Keep {
		if tail[i].Role != k.Role || tail[i].Content != k.Content {
			t.Fatalf("boundary keep[%d]=%+v does not match in-memory tail %+v", i, k, tail[i])
		}
	}
}

// Without a sessionID/workDir, autoCompact must NOT touch any session log
// (one-shot callers, tests, sub-agents) — behaviour stays as before.
func TestAutoCompactNoSessionNoBoundary(t *testing.T) {
	conv := conversation.NewManager()
	for range 6 {
		conv.AddUserMessage("OLD-PREFIX " + bigMsg(3000))
		conv.AddAssistantMessage("OLD-REPLY " + bigMsg(3000))
	}
	conv.AddUserMessage("RECENT-A")
	conv.AddAssistantMessage("RECENT-B")

	workDir := t.TempDir()
	client := &stubSummaryClient{summary: "S"}
	// Empty sessionID → no persistence.
	if _, err := autoCompact(context.Background(), conv, client, workDir, "", 200000, nil, nil); err != nil {
		t.Fatalf("autoCompact error: %v", err)
	}
	// No session file should have been created under any id.
	msgs := session.LoadSession(workDir, "anything")
	if len(msgs) != 0 {
		t.Fatalf("expected no session log written when sessionID empty, got %d", len(msgs))
	}
}

// autoCompact must summarize only messages[:keepStart]; the kept tail must NOT
// appear in the prompt handed to the summarizer.
func TestAutoCompactSummaryOnlyCoversPrefix(t *testing.T) {
	conv := conversation.NewManager()
	for range 6 {
		conv.AddUserMessage("PREFIX-ONLY-CONTENT " + bigMsg(3000))
		conv.AddAssistantMessage("PREFIX-ONLY-REPLY " + bigMsg(3000))
	}
	conv.AddUserMessage("TAIL-ONLY-MARKER")
	conv.AddAssistantMessage("TAIL-ONLY-REPLY")

	client := &stubSummaryClient{summary: "S"}
	if _, err := autoCompact(context.Background(), conv, client, "", "", 200000, nil, nil); err != nil {
		t.Fatalf("autoCompact error: %v", err)
	}
	if !client.streamCalled {
		t.Fatalf("summarizer was never called")
	}
	if strings.Contains(client.lastPrompt, "TAIL-ONLY-MARKER") {
		t.Errorf("summary prompt must not include the kept tail, but it did")
	}
	if !strings.Contains(client.lastPrompt, "PREFIX-ONLY-CONTENT") {
		t.Errorf("summary prompt must include the summarized prefix, but it did not")
	}
}

// computeKeepStartIndex must never split a tool_use ↔ tool_result pair: if the
// budget boundary lands on the user message carrying tool_results, it must move
// back to include the assistant tool_use message that produced them.
func TestComputeKeepStartIndexDoesNotSplitToolPair(t *testing.T) {
	conv := conversation.NewManager()
	// Large prefix so keepStart > 0.
	for range 8 {
		conv.AddUserMessage(bigMsg(3000))
		conv.AddAssistantMessage(bigMsg(3000))
	}
	// A tool_use / tool_result pair near the tail. The tool_result is a big
	// message so the budget boundary is likely to land right on it.
	conv.AddToolUseMessage("calling tool", "tu-1", "ReadFile", map[string]any{"path": "/x"})
	conv.AddToolResultMessage("tu-1", bigMsg(9000), false)
	msgs := conv.GetMessages()

	keepStart := computeKeepStartIndex(msgs)
	if keepStart <= 0 || keepStart >= len(msgs) {
		t.Fatalf("keepStart=%d out of expected range (0, %d)", keepStart, len(msgs))
	}
	// The boundary message must not be a lone tool_result whose matching
	// tool_use was left in the summarized prefix.
	if hasToolResults(msgs[keepStart]) {
		t.Fatalf("keepStart landed on a tool_result message (orphaned); keepStart=%d", keepStart)
	}
	// Verify the pair is whole inside the kept tail: walk it and ensure every
	// tool_result has a preceding tool_use within the kept slice.
	keep := msgs[keepStart:]
	openUses := map[string]bool{}
	for _, m := range keep {
		for _, tu := range m.ToolUses {
			openUses[tu.ToolUseID] = true
		}
		for _, tr := range m.ToolResults {
			if !openUses[tr.ToolUseID] {
				t.Errorf("tool_result %s in kept tail has no matching tool_use in tail (pair split)", tr.ToolUseID)
			}
		}
	}
}

// When the conversation is too short to have any summarizable prefix (keepStart
// <= 0), autoCompact must degrade to a no-op: no summarization, conversation
// left untouched.
func TestAutoCompactDegradesWhenTooFewMessages(t *testing.T) {
	conv := conversation.NewManager()
	conv.AddUserMessage("just one")
	conv.AddAssistantMessage("two")
	before := conv.GetMessages()

	client := &stubSummaryClient{summary: "S"}
	msg, err := autoCompact(context.Background(), conv, client, "", "", 200000, nil, nil)
	if err != nil {
		t.Fatalf("autoCompact error: %v", err)
	}
	if msg != "" {
		t.Errorf("expected no-op (empty message) for too-few messages, got %q", msg)
	}
	if client.streamCalled {
		t.Errorf("summarizer should not be called when degrading to no-op")
	}
	after := conv.GetMessages()
	if len(before) != len(after) {
		t.Errorf("conversation changed during no-op: before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		if before[i].Content != after[i].Content {
			t.Errorf("message %d mutated during no-op", i)
		}
	}
}

func msgContents(msgs []conversation.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		c := m.Content
		if len(c) > 40 {
			c = c[:40] + "..."
		}
		out[i] = c
	}
	return out
}
