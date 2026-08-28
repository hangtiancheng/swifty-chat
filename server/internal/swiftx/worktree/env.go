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
	"bytes"
	"context"
	"os"
	"os/exec"
)

// gitNoPromptEnv returns the base environment for every git subprocess this package spawns, with
// two safety knobs appended:
//
// GIT_TERMINAL_PROMPT=0: prevents git from opening /dev/tty for credential
// prompts (which would hang the CLI).
// GIT_ASKPASS="": disables askpass GUI programs (same outcome via
// a different code path).
//
// Together with Stdin = nil on the *exec.Cmd, this closes every channel through which git could
// block on interactive input.
func gitNoPromptEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=")
}

// runGit invokes `git <args.>` inside dir with stdin closed and the no-prompt environment applied.
// Returns stdout, stderr, and the exit code (or -1 if the process didn't run). Never throws on
// non-zero exit; the caller decides whether code != 0 is an error in context.
//
// ctx propagates cancellation: cancelling ctx kills the git subprocess.
func runGit(ctx context.Context, dir string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitNoPromptEnv()
	cmd.Stdin = nil
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err == nil {
		return stdout, stderr, 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return stdout, stderr, ee.ExitCode()
	}
	// Process failed to start (git not on PATH, dir doesn't exist, etc.).
	return stdout, stderr, -1
}
