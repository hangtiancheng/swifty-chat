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
	"os/exec"
	"strings"
	"testing"
)

func TestGitNoPromptEnv(t *testing.T) {
	env := gitNoPromptEnv()
	hasPrompt := false
	hasAskpass := false
	for _, kv := range env {
		if kv == "GIT_TERMINAL_PROMPT=0" {
			hasPrompt = true
		}
		if kv == "GIT_ASKPASS=" {
			hasAskpass = true
		}
	}
	if !hasPrompt {
		t.Error("gitNoPromptEnv missing GIT_TERMINAL_PROMPT=0")
	}
	if !hasAskpass {
		t.Error(`gitNoPromptEnv missing GIT_ASKPASS=""`)
	}
}

func TestRunGit_Version(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	stdout, _, code := runGit(context.Background(), t.TempDir(), "--version")
	if code != 0 {
		t.Fatalf("git --version exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "git version") {
		t.Errorf("git --version stdout = %q, want substring 'git version'", stdout)
	}
}

func TestRunGit_NonZeroExit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	// Run git status in a non-repo dir → non-zero exit, no panic.
	_, stderr, code := runGit(context.Background(), t.TempDir(), "status")
	if code == 0 {
		t.Errorf("git status in non-repo: expected non-zero exit, got 0")
	}
	if stderr == "" {
		t.Errorf("git status in non-repo: expected stderr message, got empty")
	}
}

func TestRunGit_ContextCancel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	_, _, code := runGit(ctx, t.TempDir(), "--version")
	// Cancelled context kills the process; exit code is -1 (didn't run) or
	// a non-zero signal-derived code. Either way, not 0.
	if code == 0 {
		t.Errorf("cancelled ctx: expected non-zero exit, got 0")
	}
}
