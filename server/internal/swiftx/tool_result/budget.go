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

package tool_result

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

const (
	// MessageAggregateLimit is the maximum total character count for all tool
	// results in a single message. Individual result size is capped by
	// tools.MaxOutputChars; this limit governs the aggregate — when multiple
	// tools are called in parallel within one turn, each result may be under
	// the per-result threshold yet their sum can still overflow the context.
	MessageAggregateLimit = 200000
)

// SpillDir returns the spill directory: isolated per session under
// .swiftx/sessions/<session-id>/tool-results. When sessionID is empty
// (one-shot invocations, tests), it falls back to "default".
func SpillDir(workDir, sessionID string) string {
	if sessionID == "" {
		sessionID = "default"
	}
	return filepath.Join(workDir, ".swiftx", "sessions", sessionID, "tool-results")
}

// ApplyBudget enforces the aggregate budget before a batch of tool results
// enters the conversation history: when the total character count exceeds
// MessageAggregateLimit, results are spilled to disk one by one starting from
// the largest, replaced in-place with a preview, until the total is back
// within the limit.
//
// tool_use_ids in exempt are excluded from spilling: readback results of
// spill files (re-spilling would prevent the model from ever seeing the full
// text), and results already individually spilled in this turn. If all
// results are exempt, the overage is accepted.
func ApplyBudget(results []conversation.ToolResultBlock, exempt map[string]bool, workDir, sessionID string) {
	total := 0
	for i := range results {
		total += len(results[i].Content)
	}
	if total <= MessageAggregateLimit {
		return
	}

	spillDir := SpillDir(workDir, sessionID)

	// Sort by content length descending: spill the largest first so that
	// the fewest results need to be touched to get back under the limit.
	order := make([]int, len(results))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return len(results[order[a]].Content) > len(results[order[b]].Content)
	})

	for _, idx := range order {
		if total <= MessageAggregateLimit {
			break
		}
		r := &results[idx]
		if exempt[r.ToolUseID] {
			continue
		}
		if len(r.Content) <= previewSize {
			// Results shorter than the preview gain nothing from spilling.
			continue
		}
		path, err := writeSpill(spillDir, r.ToolUseID, r.Content)
		if err != nil {
			// On write failure, keep the original content. The message is
			// about to be finalized into history; there will be no retry.
			continue
		}
		preview := buildSpillPreview(r.Content, path)
		total -= len(r.Content) - len(preview)
		r.Content = preview
	}
}

// IsSpillReadback reports whether a tool call is reading back a file from the
// spill directory. Such results must not be spilled: writing the model's
// freshly-read content back to disk and replacing it with a preview would
// prevent the model from ever seeing the full text, creating a readback-spill
// loop.
func IsSpillReadback(toolName string, args map[string]any, workDir, sessionID string) bool {
	if toolName != "ReadFile" {
		return false
	}
	raw, _ := args["file_path"].(string)
	if raw == "" {
		return false
	}
	absSpillDir, err := filepath.Abs(SpillDir(workDir, sessionID))
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return false
	}
	return strings.HasPrefix(abs, absSpillDir)
}

// previewSize is the maximum character count for the persisted preview,
// taking the first 2KB to balance readability and space usage.
const previewSize = 2000

// buildSpillPreview constructs the persisted replacement text containing a
// 2KB preview. Identical input always produces byte-identical output — once
// the replacement text enters the conversation history it is never modified;
// format changes only affect newly produced results.
func buildSpillPreview(content string, path string) string {
	sizeKB := len(content) / 1024
	preview := content
	hasMore := false
	if len(preview) > previewSize {
		preview = preview[:previewSize]
		hasMore = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<persisted-output>\n")
	fmt.Fprintf(&b, "Output too large (%dKB). Full content saved to:\n%s\n\n", sizeKB, path)
	fmt.Fprintf(&b, "Preview (first 2KB):\n%s", preview)
	if hasMore {
		b.WriteString("\n...")
	}
	b.WriteString("\n</persisted-output>")
	return b.String()
}

func writeSpill(dir, toolUseID, content string) (string, error) {
	if toolUseID == "" {
		return "", fmt.Errorf("empty tool_use_id")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, toolUseID+".txt")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return path, nil
		}
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return path, nil
}

// PersistLargeResult spills oversized tool output to disk and returns a
// preview. Called by the agent when a tool result enters the conversation
// history.
func PersistLargeResult(workDir, sessionID, toolUseID, content string) string {
	dir := SpillDir(workDir, sessionID)
	path, err := writeSpill(dir, toolUseID, content)
	if err != nil {
		return content
	}
	return buildSpillPreview(content, path)
}
