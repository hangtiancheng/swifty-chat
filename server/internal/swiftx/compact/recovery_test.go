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
	"strings"
	"testing"
	"time"
)

func TestRecoveryStateNilSafe(t *testing.T) {
	var s *RecoveryState
	s.RecordFileRead("/x", "ignored")
	s.RecordSkillInvocation("y", "ignored")
	if got := BuildRecoveryAttachment(s, nil); got != "" {
		t.Fatalf("expected empty attachment for nil state + no tools, got %q", got)
	}
}

func TestBuildRecoveryAttachmentEmits(t *testing.T) {
	s := NewRecoveryState()
	s.RecordFileRead("/tmp/a.go", "package a\n")
	s.RecordSkillInvocation("planner", "step 1\nstep 2\n")
	schemas := []map[string]any{
		{"name": "ReadFile", "description": "Read a file and return its contents.\nWith line numbers."},
		{"name": "Bash", "description": ""},
	}

	out := BuildRecoveryAttachment(s, schemas)
	if !strings.Contains(out, "/tmp/a.go") {
		t.Errorf("expected file path in attachment, got: %s", out)
	}
	if !strings.Contains(out, "planner") {
		t.Errorf("expected skill name in attachment, got: %s", out)
	}
	if !strings.Contains(out, "- ReadFile — Read a file and return its contents.") {
		t.Errorf("expected tool listing with first-line description, got: %s", out)
	}
	if !strings.Contains(out, "- Bash") {
		t.Errorf("expected bare tool listing without description, got: %s", out)
	}
	if !strings.Contains(out, "Note") {
		t.Errorf("expected closing note about not guessing from summary, got: %s", out)
	}
}

func TestRecoveryFileLimitAndOrder(t *testing.T) {
	s := NewRecoveryState()
	// Record 7 files spread in time so newest-first ordering is observable.
	base := time.Now().Add(-time.Hour)
	for i := range 7 {
		path := "/f" + string(rune('0'+i))
		s.RecordFileRead(path, "x")
		// Force-set timestamps so ordering is deterministic.
		rec := s.files[path]
		rec.Timestamp = base.Add(time.Duration(i) * time.Minute)
		s.files[path] = rec
	}
	out := BuildRecoveryAttachment(s, nil)
	// Only the 5 most-recent should appear.
	if strings.Count(out, "###") != 5 {
		t.Fatalf("expected 5 file sections, got: %d in %s", strings.Count(out, "###"), out)
	}
	// Newest first: f6 must come before f2.
	idxNew := strings.Index(out, "/f6")
	idxOld := strings.Index(out, "/f2")
	if idxNew < 0 || idxOld < 0 || idxNew > idxOld {
		t.Errorf("expected newest file (/f6) to appear before older (/f2); got idx new=%d old=%d", idxNew, idxOld)
	}
}

func TestRecoveryTruncatesPerFile(t *testing.T) {
	huge := strings.Repeat("x", int(float64(RecoveryTokensPerFile)*recoveryCharsPerToken)*3)
	s := NewRecoveryState()
	s.RecordFileRead("/big", huge)
	out := BuildRecoveryAttachment(s, nil)
	if !strings.Contains(out, "(content truncated)") {
		t.Errorf("expected truncation marker for oversize file, got prefix: %s", out[:200])
	}
}

func TestRecoverySkillsBudget(t *testing.T) {
	s := NewRecoveryState()
	// 6 skills × 5K-token bodies ⇒ total 30K, must stop at 25K budget.
	bodyChars := int(float64(RecoveryTokensPerSkill) * recoveryCharsPerToken)
	body := strings.Repeat("y", bodyChars)
	base := time.Now()
	for i := range 6 {
		name := "skill-" + string(rune('0'+i))
		s.RecordSkillInvocation(name, body)
		rec := s.skills[name]
		rec.Timestamp = base.Add(time.Duration(i) * time.Minute)
		s.skills[name] = rec
	}
	out := BuildRecoveryAttachment(s, nil)
	// 25K / 5K per skill = 5 max.
	emitted := strings.Count(out, "### skill-")
	if emitted < 1 || emitted > 5 {
		t.Errorf("expected at most 5 skills under budget, emitted %d", emitted)
	}
}
