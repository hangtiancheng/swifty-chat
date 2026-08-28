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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreesDir(t *testing.T) {
	got := WorktreesDir("/tmp/repo")
	want := filepath.Join("/tmp/repo", ".swiftx", "worktrees")
	if got != want {
		t.Errorf("WorktreesDir = %q, want %q", got, want)
	}
}

func TestWorktreePathFor_FlattensNestedSlugs(t *testing.T) {
	got := WorktreePathFor("/tmp/repo", "team/alice")
	want := filepath.Join("/tmp/repo", ".swiftx", "worktrees", "team+alice")
	if got != want {
		t.Errorf("WorktreePathFor(team/alice) = %q, want %q", got, want)
	}
}

func TestGetOrCreateWorktree_RoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	repo := t.TempDir()
	initBareRepoWithCommit(t, repo)

	ctx := context.Background()

	// First call: creates a new worktree.
	r1, err := getOrCreateWorktree(ctx, repo, "feature-x")
	if err != nil {
		t.Fatalf("first getOrCreateWorktree: %v", err)
	}
	if r1.Existed {
		t.Errorf("first call: Existed=true, want false")
	}
	if r1.WorktreeBranch != "worktree-feature-x" {
		t.Errorf("WorktreeBranch = %q, want worktree-feature-x", r1.WorktreeBranch)
	}
	if !strings.HasSuffix(r1.WorktreePath, filepath.Join(".swiftx", "worktrees", "feature-x")) {
		t.Errorf("WorktreePath = %q, missing expected suffix", r1.WorktreePath)
	}
	if !IsValidGitSha(r1.HeadCommit) {
		t.Errorf("HeadCommit = %q, not a valid SHA", r1.HeadCommit)
	}
	if _, err := os.Stat(filepath.Join(r1.WorktreePath, ".git")); err != nil {
		t.Errorf(".git pointer not present in worktree: %v", err)
	}

	// Second call same slug: fast-resume returns Existed=true.
	r2, err := getOrCreateWorktree(ctx, repo, "feature-x")
	if err != nil {
		t.Fatalf("second getOrCreateWorktree: %v", err)
	}
	if !r2.Existed {
		t.Errorf("second call: Existed=false, want true (fast resume)")
	}
	if r2.HeadCommit != r1.HeadCommit {
		t.Errorf("resume HeadCommit = %q, want same as create (%q)", r2.HeadCommit, r1.HeadCommit)
	}

	// Remove the worktree dir manually; next call should go through full
	// creation path again (and -B should reset the orphan branch).
	if out, err := exec.Command("git", "-C", repo, "worktree", "remove", "--force", r1.WorktreePath).CombinedOutput(); err != nil {
		t.Fatalf("cleanup git worktree remove: %v\n%s", err, out)
	}
	r3, err := getOrCreateWorktree(ctx, repo, "feature-x")
	if err != nil {
		t.Fatalf("third getOrCreateWorktree (after remove): %v", err)
	}
	if r3.Existed {
		t.Errorf("third call (after remove): Existed=true, want false")
	}
}

func TestGetOrCreateWorktree_NestedSlug(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	repo := t.TempDir()
	initBareRepoWithCommit(t, repo)
	r, err := getOrCreateWorktree(context.Background(), repo, "team-refactor/alice")
	if err != nil {
		t.Fatalf("getOrCreateWorktree: %v", err)
	}
	if r.WorktreeBranch != "worktree-team-refactor+alice" {
		t.Errorf("WorktreeBranch = %q, want worktree-team-refactor+alice", r.WorktreeBranch)
	}
	if !strings.HasSuffix(r.WorktreePath, filepath.Join(".swiftx", "worktrees", "team-refactor+alice")) {
		t.Errorf("WorktreePath flatten mismatch: %q", r.WorktreePath)
	}
}
