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
	"testing"
	"time"
)

func TestIsEphemeralSlug(t *testing.T) {
	tests := []struct {
		slug     string
		expected bool
	}{
		{"agent-a1234567", true},
		{"agent-aabcdef0", true},
		{"wf_12345678-abc-1", true},
		{"wf_12345678-abc-42", true},
		{"wf-1", true},
		{"wf-99", true},
		{"bridge-abc", true},
		{"bridge-abc_def-ghi", true},
		{"job-mytemplate-12345678", true},
		// Should NOT match
		{"my-feature", false},
		{"agent-too-long", false},
		{"agent-a123", false},     // too short
		{"agent-aGGGGGGG", false}, // non-hex
		{"wf_short", false},
	}
	for _, tt := range tests {
		got := isEphemeralSlug(tt.slug)
		if got != tt.expected {
			t.Errorf("isEphemeralSlug(%q) = %v, want %v", tt.slug, got, tt.expected)
		}
	}
}

func TestCleanupStaleAgentWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	// Add a fake remote so --not --remotes works (worktree HEAD is
	// reachable from the remote, so rev-list returns empty)
	bare := t.TempDir()
	exec.Command("git", "init", "--bare", bare).Run()
	exec.Command("git", "-C", repo, "remote", "add", "origin", bare).Run()
	exec.Command("git", "-C", repo, "push", "origin", "HEAD").Run()

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repo)

	// Create an ephemeral worktree
	result, err := CreateAgentWorktree(context.Background(), "agent-aaaaaaaa")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Set mtime to 25 hours ago
	past := time.Now().Add(-25 * time.Hour)
	os.Chtimes(result.WorktreePath, past, past)

	// Cleanup with cutoff = 24 hours ago should remove it
	cutoff := time.Now().Add(-24 * time.Hour)
	removed := CleanupStaleAgentWorktrees(context.Background(), cutoff)
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	// Directory should be gone
	if _, err := os.Stat(result.WorktreePath); !os.IsNotExist(err) {
		t.Fatal("stale worktree should be removed")
	}
}

func TestCleanupStaleAgentWorktrees_SkipsUserNamed(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repo)

	// Create a user-named worktree (not ephemeral)
	_, err := getOrCreateWorktree(context.Background(), repo, "my-feature")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	wtPath := WorktreePathFor(repo, "my-feature")

	// Set mtime to past
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(wtPath, past, past)

	// Cleanup should NOT remove user-named worktree
	cutoff := time.Now().Add(-24 * time.Hour)
	removed := CleanupStaleAgentWorktrees(context.Background(), cutoff)
	if removed != 0 {
		t.Fatal("user-named worktree should not be cleaned up")
	}

	if _, err := os.Stat(wtPath); err != nil {
		t.Fatal("user-named worktree should still exist")
	}
}

func TestCleanupStaleAgentWorktrees_SkipsDirtyWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repo)

	result, err := CreateAgentWorktree(context.Background(), "agent-abbbbbbb")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Make it dirty
	os.WriteFile(filepath.Join(result.WorktreePath, "dirty.txt"), []byte("dirty"), 0o644)
	exec.Command("git", "-C", result.WorktreePath, "add", ".").Run()

	// Set mtime to past
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(result.WorktreePath, past, past)

	// Cleanup should skip the dirty worktree
	cutoff := time.Now().Add(-24 * time.Hour)
	removed := CleanupStaleAgentWorktrees(context.Background(), cutoff)
	if removed != 0 {
		t.Fatal("dirty worktree should not be cleaned up")
	}
}
