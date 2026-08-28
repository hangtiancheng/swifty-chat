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
	"fmt"
	"regexp"
	"strings"
)

// MaxWorktreeSlugLength caps the worktree name at 64 characters.
const MaxWorktreeSlugLength = 64

// validWorktreeSlugSegment is the per-segment allowlist applied AFTER splitting the slug on '/'.
var validWorktreeSlugSegment = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateWorktreeSlug validates a worktree slug to prevent path traversal and directory escape.
// The slug is joined into `.swiftx/worktrees/<slug>` via filepath.Join, which normalizes `.`
// segments — so `./././target` would escape the worktrees directory. Similarly, an absolute path
// (leading `/` or `C:\`) would discard the prefix entirely.
//
// Forward slashes are allowed for nesting (e.g. `asm/feature-foo`); each segment is validated
// independently against the allowlist, so `.` / `.` segments and drive-spec characters are still
// rejected.
//
// Returns synchronously — callers rely on this running before any side effects (git commands, hook
// execution, chdir).
func ValidateWorktreeSlug(slug string) error {
	if len(slug) > MaxWorktreeSlugLength {
		return fmt.Errorf(
			"Invalid worktree name: must be %d characters or fewer (got %d)",
			MaxWorktreeSlugLength, len(slug),
		)
	}
	// Leading or trailing `/` would make filepath.Join produce an absolute path or a dangling segment.
	// Splitting and validating each segment rejects both (empty segments fail the regex) while
	// allowing `user/feature`.
	for segment := range strings.SplitSeq(slug, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf(
				`Invalid worktree name %q: must not contain "." or ".." path segments`,
				slug,
			)
		}
		if !validWorktreeSlugSegment.MatchString(segment) {
			return fmt.Errorf(
				`Invalid worktree name %q: each "/"-separated segment must be non-empty and contain only letters, digits, dots, underscores, and dashes`,
				slug,
			)
		}
	}
	return nil
}

// FlattenSlug flattens nested slugs (`user/feature` → `user+feature`) for both
// the branch name and the directory path. Nesting in either location is
// unsafe:
// git refs: `worktree-user` (file) vs `worktree-user/feature` (needs dir)
// is a D/F conflict that git rejects.
// directory: `.swiftx/worktrees/user/feature/` lives inside the `user`
// worktree; `git worktree remove` on the parent deletes children with
// uncommitted work.
//
// `+` is valid in git branch names and filesystem paths but NOT in the slug-segment allowlist
// ([a-zA-Z0-9._-]), so the mapping is injective.
func FlattenSlug(slug string) string {
	return strings.ReplaceAll(slug, "/", "+")
}

// WorktreeBranchName returns the git branch name for the worktree associated with slug. Format:
// "worktree-<flattenedSlug>".
func WorktreeBranchName(slug string) string {
	return "worktree-" + FlattenSlug(slug)
}
