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
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WorktreesDir is where all Swiftx-managed worktrees live: a single directory inside the repo root
// that's already in .gitignore.
func WorktreesDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".swiftx", "worktrees")
}

// WorktreePathFor returns the worktree directory path for slug, after flattening nested slugs
// (`team/alice` → `team+alice`).
func WorktreePathFor(repoRoot, slug string) string {
	return filepath.Join(WorktreesDir(repoRoot), FlattenSlug(slug))
}

// CreateResult is the outcome of getOrCreateWorktree — Existed=true means we fast-resumed an
// existing worktree (skipped `git worktree add` and performPostCreationSetup); Existed=false means
// we just created it and the caller still needs to run post-creation setup.
type CreateResult struct {
	WorktreePath   string
	WorktreeBranch string
	HeadCommit     string
	BaseBranch     string // empty when Existed=true (resume path doesn't compute baseBranch)
	Existed        bool
}

// getOrCreateWorktree creates a new git worktree for the given slug under
// <repoRoot>/.swiftx/worktrees/, or resumes it if it already exists.
//
// Fast-resume path: ReadWorktreeHeadSha reads the .git pointer file directly (no subprocess, no
// upward walk). On a 16M-object repo this saves ~6-8s of `git fetch` commit-graph scan that runs on
// every resume otherwise.
//
// Create path: resolves the default branch via filesystem-only reads (no `git fetch` when
// origin/<default> is already locally known), then runs `git worktree add -B worktree-<flat> <path>
// <baseBranch>`.
//
// `-B` (uppercase, not `-b`): resets any orphan branch left behind by a
// removed worktree dir. Saves a `git branch -D` subprocess on every create.
func getOrCreateWorktree(ctx context.Context, repoRoot, slug string) (*CreateResult, error) {
	worktreePath := WorktreePathFor(repoRoot, slug)
	worktreeBranch := WorktreeBranchName(slug)

	// Fast-resume path: existing worktree → skip fetch and add.
	existingHead, err := ReadWorktreeHeadSha(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("read worktree HEAD: %w", err)
	}
	if existingHead != "" {
		return &CreateResult{
			WorktreePath:   worktreePath,
			WorktreeBranch: worktreeBranch,
			HeadCommit:     existingHead,
			Existed:        true,
		}, nil
	}

	if err := os.MkdirAll(WorktreesDir(repoRoot), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir worktrees dir: %w", err)
	}

	// Resolve baseBranch + baseSha. If origin/<default> already exists locally, skip fetch — in large
	// repos fetch burns ~6-8s on a local commit-graph scan before even hitting the network. A slightly
	// stale base is fine; the user can pull in the worktree if they want latest.
	defaultBranch, err := GetDefaultBranch(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("get default branch: %w", err)
	}
	gitDir, err := ResolveGitDir(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve git dir: %w", err)
	}
	var baseBranch, baseSha string
	if gitDir != "" {
		baseSha, _ = ResolveRef(gitDir, "refs/remotes/origin/"+defaultBranch)
	}
	if baseSha != "" {
		baseBranch = "origin/" + defaultBranch
	} else {
		// origin/<default> isn't locally known — try `git fetch origin <default>`. If that fails
		// (offline, no remote), fall back to HEAD so we at least branch from the working tree's current
		// commit instead of erroring out.
		_, _, fetchCode := runGit(ctx, repoRoot, "fetch", "origin", defaultBranch)
		if fetchCode == 0 {
			baseBranch = "origin/" + defaultBranch
		} else {
			baseBranch = "HEAD"
		}
		// Resolve the SHA for the chosen baseBranch via subprocess — resolveRef can't find FETCH_HEAD or
		// HEAD in worktree-shared commonDir without more work.
		stdout, _, shaCode := runGit(ctx, repoRoot, "rev-parse", baseBranch)
		if shaCode != 0 {
			return nil, fmt.Errorf(`failed to resolve base branch %q: git rev-parse failed`, baseBranch)
		}
		baseSha = trimNewline(stdout)
	}

	// `-B` (capital) resets any orphan branch left behind by a removed worktree dir; `-b` would error
	// out.
	_, stderr, code := runGit(ctx, repoRoot, "worktree", "add", "-B", worktreeBranch, worktreePath, baseBranch)
	if code != 0 {
		return nil, fmt.Errorf("failed to create worktree: %s", stderr)
	}

	return &CreateResult{
		WorktreePath:   worktreePath,
		WorktreeBranch: worktreeBranch,
		HeadCommit:     baseSha,
		BaseBranch:     baseBranch,
		Existed:        false,
	}, nil
}

// trimNewline strips trailing CR/LF; equivalent to .trim on a single-line command stdout. Kept
// local to avoid importing strings just for this.
func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
