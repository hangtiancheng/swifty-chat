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

package worktree

import (
	"strings"
	"testing"
)

func TestValidateWorktreeSlug_Valid(t *testing.T) {
	cases := []string{
		"a",
		"my-feature",
		"v1.0",
		"v1.0.0-rc1",
		"team-refactor/alice",
		"asm/feature-foo",
		"agent-a1234567",
		"wf_12345678-abc-1",
	}
	for _, slug := range cases {
		if err := ValidateWorktreeSlug(slug); err != nil {
			t.Errorf("ValidateWorktreeSlug(%q) = %v, want nil", slug, err)
		}
	}
}

func TestValidateWorktreeSlug_Invalid(t *testing.T) {
	cases := []struct {
		slug    string
		wantMsg string
	}{
		{"", `each "/"-separated segment must be non-empty`},
		{strings.Repeat("a", 65), "must be 64 characters or fewer"},
		{".", `must not contain "." or ".."`},
		{"..", `must not contain "." or ".."`},
		{"foo/..", `must not contain "." or ".."`},
		{"foo/.", `must not contain "." or ".."`},
		{"../escape", `must not contain "." or ".."`},
		{"/leading-slash", `each "/"-separated segment must be non-empty`},
		{"trailing-slash/", `each "/"-separated segment must be non-empty`},
		{"foo bar", `each "/"-separated segment must be non-empty and contain only letters`},
		{"foo$bar", `each "/"-separated segment must be non-empty and contain only letters`},
		// `+` is the flatten target, so we must reject it as input to keep the
		// flattenSlug mapping injective.
		{"foo+bar", `each "/"-separated segment must be non-empty and contain only letters`},
	}
	for _, c := range cases {
		err := ValidateWorktreeSlug(c.slug)
		if err == nil {
			t.Errorf("ValidateWorktreeSlug(%q) = nil, want error containing %q", c.slug, c.wantMsg)
			continue
		}
		if !strings.Contains(err.Error(), c.wantMsg) {
			t.Errorf("ValidateWorktreeSlug(%q) = %v, want error containing %q", c.slug, err, c.wantMsg)
		}
	}
}

func TestFlattenSlug(t *testing.T) {
	cases := map[string]string{
		"my-feature":          "my-feature",
		"team/alice":          "team+alice",
		"team-refactor/alice": "team-refactor+alice",
		"a/b/c":               "a+b+c",
		"v1.0":                "v1.0",
	}
	for in, want := range cases {
		if got := FlattenSlug(in); got != want {
			t.Errorf("FlattenSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorktreeBranchName(t *testing.T) {
	cases := map[string]string{
		"my-feature":          "worktree-my-feature",
		"team/alice":          "worktree-team+alice",
		"team-refactor/alice": "worktree-team-refactor+alice",
		"agent-a1234567":      "worktree-agent-a1234567",
	}
	for in, want := range cases {
		if got := WorktreeBranchName(in); got != want {
			t.Errorf("WorktreeBranchName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaxWorktreeSlugLengthConstant(t *testing.T) {
	if MaxWorktreeSlugLength != 64 {
		t.Errorf("MaxWorktreeSlugLength = %d, want 64", MaxWorktreeSlugLength)
	}
}
