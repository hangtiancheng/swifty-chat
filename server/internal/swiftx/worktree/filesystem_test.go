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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initBareRepoWithCommit creates a regular git repo at root with one commit on
// the default branch. Returns the HEAD SHA of that commit. Skips the test if
// git isn't on PATH.
func initBareRepoWithCommit(t *testing.T, root string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	runOrSkip := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runOrSkip("init", "-q")
	runOrSkip("config", "user.email", "ch14@example.com")
	runOrSkip("config", "user.name", "ch14")
	runOrSkip("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runOrSkip("add", ".")
	runOrSkip("-c", "commit.gpgsign=false", "commit", "-q", "-m", "init")
	return runOrSkip("rev-parse", "HEAD")
}

func TestIsSafeRefName(t *testing.T) {
	good := []string{"main", "feature/foo", "release-1.2.3+build", "dep@cy"}
	for _, n := range good {
		if !IsSafeRefName(n) {
			t.Errorf("IsSafeRefName(%q) = false, want true", n)
		}
	}
	bad := []string{"", "-foo", "/abs", "..", "foo/..", "foo/./bar", "foo bar", "foo$bar", "foo\n", "foo;rm"}
	for _, n := range bad {
		if IsSafeRefName(n) {
			t.Errorf("IsSafeRefName(%q) = true, want false", n)
		}
	}
}

func TestIsValidGitSha(t *testing.T) {
	if !IsValidGitSha("0123456789abcdef0123456789abcdef01234567") {
		t.Error("40-hex should be valid SHA-1")
	}
	if !IsValidGitSha(strings.Repeat("a", 64)) {
		t.Error("64-hex should be valid SHA-256")
	}
	if IsValidGitSha("01234567") {
		t.Error("abbreviated SHA should be rejected")
	}
	if IsValidGitSha("0123456789ABCDEF0123456789ABCDEF01234567") {
		t.Error("uppercase hex should be rejected (git writes lowercase)")
	}
}

func TestResolveGitDir_RegularRepo(t *testing.T) {
	root := t.TempDir()
	initBareRepoWithCommit(t, root)
	gitDir, err := ResolveGitDir(root)
	if err != nil {
		t.Fatalf("ResolveGitDir error: %v", err)
	}
	want := filepath.Join(root, ".git")
	if gitDir != want {
		t.Errorf("ResolveGitDir = %q, want %q", gitDir, want)
	}
}

func TestResolveGitDir_NotARepo(t *testing.T) {
	root := t.TempDir()
	gitDir, err := ResolveGitDir(root)
	if err != nil {
		t.Fatalf("ResolveGitDir error: %v", err)
	}
	if gitDir != "" {
		t.Errorf("ResolveGitDir = %q, want empty (no .git)", gitDir)
	}
}

func TestReadWorktreeHeadSha_RoundTrip(t *testing.T) {
	root := t.TempDir()
	initBareRepoWithCommit(t, root)

	// git worktree add ../wt -b test-branch HEAD
	wtPath := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "worktree", "add", "-b", "test-branch", wtPath, "HEAD")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	got, err := ReadWorktreeHeadSha(wtPath)
	if err != nil {
		t.Fatalf("ReadWorktreeHeadSha error: %v", err)
	}
	if !IsValidGitSha(got) {
		t.Errorf("ReadWorktreeHeadSha = %q, not a valid SHA", got)
	}

	// Cross-check with `git rev-parse HEAD` in the worktree.
	rp := exec.Command("git", "rev-parse", "HEAD")
	rp.Dir = wtPath
	out, err := rp.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v\n%s", err, out)
	}
	want := strings.TrimSpace(string(out))
	if got != want {
		t.Errorf("ReadWorktreeHeadSha = %q, want %q (matches git rev-parse)", got, want)
	}
}

func TestReadWorktreeHeadSha_NonExistent(t *testing.T) {
	sha, err := ReadWorktreeHeadSha(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("ReadWorktreeHeadSha error: %v", err)
	}
	if sha != "" {
		t.Errorf("ReadWorktreeHeadSha = %q, want empty for nonexistent path", sha)
	}
}

func TestResolveRef_LooseRef(t *testing.T) {
	root := t.TempDir()
	initBareRepoWithCommit(t, root)
	gitDir, err := ResolveGitDir(root)
	if err != nil {
		t.Fatalf("ResolveGitDir error: %v", err)
	}

	// Default branch name varies (main or master); read HEAD to discover it.
	head, err := readGitHead(gitDir)
	if err != nil || head == nil || head.branch == "" {
		t.Fatalf("expected branch HEAD, got %+v err=%v", head, err)
	}
	sha, err := ResolveRef(gitDir, "refs/heads/"+head.branch)
	if err != nil {
		t.Fatalf("ResolveRef error: %v", err)
	}
	if !IsValidGitSha(sha) {
		t.Errorf("ResolveRef = %q, want a valid SHA", sha)
	}
}

func TestResolveRef_MissingRef(t *testing.T) {
	root := t.TempDir()
	initBareRepoWithCommit(t, root)
	gitDir, _ := ResolveGitDir(root)
	sha, err := ResolveRef(gitDir, "refs/heads/no-such-branch")
	if err != nil {
		t.Fatalf("ResolveRef error: %v", err)
	}
	if sha != "" {
		t.Errorf("ResolveRef = %q, want empty for missing ref", sha)
	}
}
