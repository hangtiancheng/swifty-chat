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
)

func TestHasWorktreeChanges_Clean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	// Get HEAD commit
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	head := trimNewline(string(out))

	// Clean repo should return false
	if HasWorktreeChanges(context.Background(), repo, head) {
		t.Fatal("expected no changes in clean repo")
	}
}

func TestHasWorktreeChanges_Dirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, _ := cmd.Output()
	head := trimNewline(string(out))

	// Create uncommitted file
	os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0o644)

	if !HasWorktreeChanges(context.Background(), repo, head) {
		t.Fatal("expected changes with uncommitted file")
	}
}

func TestHasWorktreeChanges_NewCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, _ := cmd.Output()
	head := trimNewline(string(out))

	// Add a new commit
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new"), 0o644)
	exec.Command("git", "-C", repo, "add", ".").Run()
	exec.Command("git", "-C", repo, "commit", "-m", "new").Run()

	if !HasWorktreeChanges(context.Background(), repo, head) {
		t.Fatal("expected changes with new commit")
	}
}

func TestHasWorktreeChanges_FailClosed(t *testing.T) {
	// Non-existent path should return true (fail-closed)
	if !HasWorktreeChanges(context.Background(), "/nonexistent-path-xyz", "abc123") {
		t.Fatal("expected true for non-existent path (fail-closed)")
	}
}

func TestCountWorktreeChanges_Clean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, _ := cmd.Output()
	head := trimNewline(string(out))

	summary := CountWorktreeChanges(context.Background(), repo, head)
	if summary == nil {
		t.Fatal("expected non-nil summary for clean repo")
	}
	if summary.ChangedFiles != 0 || summary.Commits != 0 {
		t.Fatalf("expected 0/0, got %d/%d", summary.ChangedFiles, summary.Commits)
	}
}

func TestCountWorktreeChanges_EmptyHeadCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	// Empty originalHeadCommit should return nil (fail-closed)
	summary := CountWorktreeChanges(context.Background(), repo, "")
	if summary != nil {
		t.Fatal("expected nil with empty head commit (fail-closed)")
	}
}

func TestCountWorktreeChanges_FailClosed(t *testing.T) {
	summary := CountWorktreeChanges(context.Background(), "/nonexistent-path-xyz", "abc123")
	if summary != nil {
		t.Fatal("expected nil for non-existent path (fail-closed)")
	}
}
