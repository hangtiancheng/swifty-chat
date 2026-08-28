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

package memory

import (
	"strings"
	"testing"
	"time"
)

func TestMemoryAgeDays(t *testing.T) {
	now := time.Now().UnixMilli()
	day := int64(86_400_000)
	cases := map[string]struct {
		mtimeMs int64
		want    int
	}{
		"today (now)":            {now, 0},
		"today (a few hours)":    {now - day/4, 0},
		"yesterday":              {now - day, 1},
		"47 days":                {now - day*47, 47},
		"future clamps to 0":     {now + day, 0},
		"a second in the future": {now + 1000, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := MemoryAgeDays(tc.mtimeMs); got != tc.want {
				t.Errorf("MemoryAgeDays = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMemoryAge(t *testing.T) {
	now := time.Now().UnixMilli()
	day := int64(86_400_000)
	cases := map[int64]string{
		now:           "today",
		now - day:     "yesterday",
		now - day*2:   "2 days ago",
		now - day*100: "100 days ago",
	}
	for mtime, want := range cases {
		if got := MemoryAge(mtime); got != want {
			t.Errorf("MemoryAge(%d) = %q, want %q", mtime, got, want)
		}
	}
}

func TestMemoryFreshnessText(t *testing.T) {
	now := time.Now().UnixMilli()
	day := int64(86_400_000)

	if got := MemoryFreshnessText(now); got != "" {
		t.Errorf("fresh memory should produce no warning, got: %q", got)
	}
	if got := MemoryFreshnessText(now - day); got != "" {
		t.Errorf("yesterday should produce no warning, got: %q", got)
	}
	got := MemoryFreshnessText(now - day*47)
	if !strings.Contains(got, "47 days old") {
		t.Errorf("warning should mention age, got: %q", got)
	}
	if !strings.Contains(got, "Verify against current code") {
		t.Errorf("warning should include verification reminder, got: %q", got)
	}
}

func TestMemoryFreshnessNote(t *testing.T) {
	now := time.Now().UnixMilli()
	day := int64(86_400_000)

	if got := MemoryFreshnessNote(now); got != "" {
		t.Errorf("fresh memory should produce no note, got: %q", got)
	}
	got := MemoryFreshnessNote(now - day*5)
	if !strings.HasPrefix(got, "<system-reminder>") || !strings.HasSuffix(got, "</system-reminder>\n") {
		t.Errorf("note should be wrapped in <system-reminder>...</system-reminder>\\n, got: %q", got)
	}
}
