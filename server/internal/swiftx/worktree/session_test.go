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

func TestSessionSingleton(t *testing.T) {
	// Initially nil
	if s := GetCurrentWorktreeSession(); s != nil {
		t.Fatal("expected nil session at start")
	}

	// Restore a session
	session := &WorktreeSession{
		OriginalCwd:    "/tmp/original",
		WorktreePath:   "/tmp/wt",
		WorktreeName:   "test-wt",
		WorktreeBranch: "worktree-test-wt",
		SessionID:      "sid-1",
	}
	RestoreWorktreeSession(session)
	got := GetCurrentWorktreeSession()
	if got == nil {
		t.Fatal("expected non-nil session after restore")
	}
	if got.WorktreeName != "test-wt" {
		t.Fatalf("expected name 'test-wt', got %q", got.WorktreeName)
	}

	// Restore nil clears
	RestoreWorktreeSession(nil)
	if s := GetCurrentWorktreeSession(); s != nil {
		t.Fatal("expected nil after restoring nil")
	}
}

func TestSaveLoadWorktreeSession(t *testing.T) {
	dir := t.TempDir()

	// Load from non-existent file returns nil, nil
	s, err := LoadWorktreeSession(dir)
	if err != nil || s != nil {
		t.Fatalf("expected (nil, nil), got (%v, %v)", s, err)
	}

	// Save a session
	session := &WorktreeSession{
		OriginalCwd:    "/tmp/orig",
		WorktreePath:   "/tmp/wt",
		WorktreeName:   "my-feature",
		WorktreeBranch: "worktree-my-feature",
		SessionID:      "sid-42",
	}
	if err := SaveWorktreeSession(dir, session); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, ".swiftx", "worktree_session.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file not created: %v", err)
	}

	// Load it back
	loaded, err := LoadWorktreeSession(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.WorktreeName != "my-feature" {
		t.Fatalf("expected name 'my-feature', got %q", loaded.WorktreeName)
	}
	if loaded.SessionID != "sid-42" {
		t.Fatalf("expected session ID 'sid-42', got %q", loaded.SessionID)
	}

	// Save nil deletes the file
	if err := SaveWorktreeSession(dir, nil); err != nil {
		t.Fatalf("save nil failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected session file to be deleted")
	}
}

func TestCreateWorktreeForSession(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repo)

	session, err := CreateWorktreeForSession(context.Background(), "sid-1", "my-feature", repo)
	if err != nil {
		t.Fatalf("CreateWorktreeForSession failed: %v", err)
	}
	if session.WorktreeBranch != "worktree-my-feature" {
		t.Fatalf("unexpected branch %q", session.WorktreeBranch)
	}
	if session.CreationDurationMs <= 0 {
		t.Fatal("expected positive creation duration for new worktree")
	}

	// Singleton should be set
	got := GetCurrentWorktreeSession()
	if got == nil || got.WorktreeName != "my-feature" {
		t.Fatal("singleton not set after CreateWorktreeForSession")
	}

	// Cleanup
	RestoreWorktreeSession(nil)
	os.Chdir(repo)
}

func TestKeepWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repo)

	_, err := CreateWorktreeForSession(context.Background(), "sid-1", "keep-test", repo)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	session := GetCurrentWorktreeSession()
	wtPath := session.WorktreePath

	os.Chdir(wtPath) // simulate being in worktree
	if err := KeepWorktree(repo); err != nil {
		t.Fatalf("keep failed: %v", err)
	}

	if s := GetCurrentWorktreeSession(); s != nil {
		t.Fatal("session should be nil after keep")
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatal("worktree directory should still exist after keep")
	}
}

func TestCleanupWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	repo := t.TempDir()
	initTestRepo(t, repo)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(repo)

	_, err := CreateWorktreeForSession(context.Background(), "sid-1", "cleanup-test", repo)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	session := GetCurrentWorktreeSession()
	wtPath := session.WorktreePath

	os.Chdir(wtPath)
	if err := CleanupWorktree(context.Background(), repo); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if s := GetCurrentWorktreeSession(); s != nil {
		t.Fatal("session should be nil after cleanup")
	}
	// Directory should be gone
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatal("worktree directory should be removed after cleanup")
	}
}

func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s", c, out)
		}
	}
	// Create initial commit
	f := filepath.Join(dir, "init.txt")
	os.WriteFile(f, []byte("init"), 0o644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = dir
	cmd.CombinedOutput()
}
