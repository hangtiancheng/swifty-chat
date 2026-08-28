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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

func batchOf(sizes map[string]int) []conversation.ToolResultBlock {
	var rs []conversation.ToolResultBlock
	// Fixed order to ensure test reproducibility.
	for _, id := range []string{"t1", "t2", "t3", "t4", "t5"} {
		if n, ok := sizes[id]; ok {
			rs = append(rs, conversation.ToolResultBlock{
				ToolUseID: id,
				Content:   strings.Repeat(string(id[1]), n),
			})
		}
	}
	return rs
}

func totalLen(rs []conversation.ToolResultBlock) int {
	total := 0
	for _, r := range rs {
		total += len(r.Content)
	}
	return total
}

func TestApplyBudgetUnderLimitUntouched(t *testing.T) {
	rs := batchOf(map[string]int{"t1": 40000, "t2": 40000})
	before := rs[0].Content

	ApplyBudget(rs, nil, t.TempDir(), "")

	if rs[0].Content != before {
		t.Fatal("under-limit batch must not be modified")
	}
}

func TestApplyBudgetSpillsLargestFirst(t *testing.T) {
	// 5 x 45K = 225K, exceeding the 200K limit. Spilling only the largest
	// result is sufficient to get back under the limit.
	rs := batchOf(map[string]int{"t1": 45000, "t2": 45000, "t3": 45001, "t4": 45000, "t5": 45000})
	dir := t.TempDir()

	ApplyBudget(rs, nil, dir, "")

	if got := totalLen(rs); got > MessageAggregateLimit {
		t.Fatalf("aggregate %d still over limit %d", got, MessageAggregateLimit)
	}
	// t3 is the largest and should have been spilled.
	var t3 string
	replaced := 0
	for _, r := range rs {
		if strings.HasPrefix(r.Content, "<persisted-output>") {
			replaced++
		}
		if r.ToolUseID == "t3" {
			t3 = r.Content
		}
	}
	if replaced != 1 {
		t.Fatalf("expected exactly 1 replacement, got %d", replaced)
	}
	if !strings.HasPrefix(t3, "<persisted-output>") {
		t.Fatal("largest result t3 should have been spilled")
	}
	// The spill file exists and its content is complete.
	data := filepath.Join(SpillDir(dir, ""), "t3.txt")
	if fi := mustStat(t, data); fi != 45001 {
		t.Fatalf("spill file size = %d, want 45001", fi)
	}
}

func TestApplyBudgetExemptSkipped(t *testing.T) {
	// The largest result is exempt (e.g. a spill-file readback); the next
	// largest should be spilled instead.
	rs := batchOf(map[string]int{"t1": 45000, "t2": 45000, "t3": 45001, "t4": 45000, "t5": 45000})
	exempt := map[string]bool{"t3": true}

	ApplyBudget(rs, exempt, t.TempDir(), "")

	for _, r := range rs {
		if r.ToolUseID == "t3" && strings.HasPrefix(r.Content, "<persisted-output>") {
			t.Fatal("exempt t3 must not be spilled")
		}
	}
	if got := totalLen(rs); got > MessageAggregateLimit {
		t.Fatalf("aggregate %d still over limit", got)
	}
}

func TestApplyBudgetAllExemptAcceptsOverage(t *testing.T) {
	rs := batchOf(map[string]int{"t1": 105000, "t2": 105000})
	exempt := map[string]bool{"t1": true, "t2": true}
	before1, before2 := rs[0].Content, rs[1].Content

	ApplyBudget(rs, exempt, t.TempDir(), "")

	if rs[0].Content != before1 || rs[1].Content != before2 {
		t.Fatal("all-exempt batch must be left as-is (accepted overage)")
	}
}

func TestApplyBudgetDeterministic(t *testing.T) {
	// Two calls with identical input (same disk directory) must produce
	// byte-identical preview strings.
	dir := t.TempDir()
	rs1 := batchOf(map[string]int{"t1": 45000, "t2": 45000, "t3": 45001, "t4": 45000, "t5": 45000})
	rs2 := batchOf(map[string]int{"t1": 45000, "t2": 45000, "t3": 45001, "t4": 45000, "t5": 45000})

	ApplyBudget(rs1, nil, dir, "")
	ApplyBudget(rs2, nil, dir, "")

	for i := range rs1 {
		if rs1[i].Content != rs2[i].Content {
			t.Fatalf("non-deterministic output for %s", rs1[i].ToolUseID)
		}
	}
}

func TestApplyBudgetIdempotent(t *testing.T) {
	// A batch already under the limit is a no-op on a second pass.
	dir := t.TempDir()
	rs := batchOf(map[string]int{"t1": 45000, "t2": 45000, "t3": 45001, "t4": 45000, "t5": 45000})
	ApplyBudget(rs, nil, dir, "")

	snapshot := make([]string, len(rs))
	for i, r := range rs {
		snapshot[i] = r.Content
	}

	ApplyBudget(rs, nil, dir, "")
	for i, r := range rs {
		if r.Content != snapshot[i] {
			t.Fatalf("second pass modified %s", r.ToolUseID)
		}
	}
}

func TestIsSpillReadback(t *testing.T) {
	wd := t.TempDir()
	inside := filepath.Join(SpillDir(wd, ""), "toolu_abc.txt")
	outside := filepath.Join(wd, "main.go")

	if !IsSpillReadback("ReadFile", map[string]any{"file_path": inside}, wd, "") {
		t.Fatal("ReadFile of spill file should be a readback")
	}
	if IsSpillReadback("ReadFile", map[string]any{"file_path": outside}, wd, "") {
		t.Fatal("ReadFile outside spill dir is not a readback")
	}
	if IsSpillReadback("Bash", map[string]any{"file_path": inside}, wd, "") {
		t.Fatal("non-ReadFile tool is never a readback")
	}
	if IsSpillReadback("ReadFile", map[string]any{}, wd, "") {
		t.Fatal("missing file_path is not a readback")
	}
}

func TestPersistLargeResultRoundTrip(t *testing.T) {
	wd := t.TempDir()
	content := strings.Repeat("x", 60000)

	preview := PersistLargeResult(wd, "", "t_big", content)

	if !strings.HasPrefix(preview, "<persisted-output>") {
		t.Fatalf("preview missing tag: %q", preview[:40])
	}
	if !strings.Contains(preview, "Preview (first 2KB)") {
		t.Fatal("preview missing preview section")
	}
	if fi := mustStat(t, filepath.Join(SpillDir(wd, ""), "t_big.txt")); fi != 60000 {
		t.Fatalf("spill file size = %d, want 60000", fi)
	}
	// A second call (file already exists) returns the identical preview.
	if again := PersistLargeResult(wd, "", "t_big", content); again != preview {
		t.Fatal("second persist must reproduce identical preview")
	}
}

func mustStat(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}
