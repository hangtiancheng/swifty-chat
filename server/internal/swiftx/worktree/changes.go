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
	"strconv"
	"strings"
)

// ChangeSummary holds the result of countWorktreeChanges.
type ChangeSummary struct {
	ChangedFiles int
	Commits      int
}

// HasWorktreeChanges returns true if the worktree has uncommitted changes or new commits since
// headCommit. Returns true on git failure (fail-closed).
func HasWorktreeChanges(ctx context.Context, worktreePath, headCommit string) bool {
	stdout, _, code := runGit(ctx, worktreePath, "status", "--porcelain")
	if code != 0 {
		return true // fail-closed
	}
	if strings.TrimSpace(stdout) != "" {
		return true
	}

	stdout, _, code = runGit(ctx, worktreePath, "rev-list", "--count", headCommit+"..HEAD")
	if code != 0 {
		return true // fail-closed
	}
	n, err := strconv.Atoi(strings.TrimSpace(stdout))
	if err != nil {
		return true // fail-closed
	}
	return n > 0
}

// CountWorktreeChanges returns a detailed change summary, or nil when state cannot be reliably
// determined. Callers that use this as a safety gate must treat nil as "unknown, assume unsafe"
// (fail-closed).
func CountWorktreeChanges(ctx context.Context, worktreePath, originalHeadCommit string) *ChangeSummary {
	stdout, _, code := runGit(ctx, worktreePath, "status", "--porcelain")
	if code != 0 {
		return nil // fail-closed
	}
	changedFiles := 0
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.TrimSpace(line) != "" {
			changedFiles++
		}
	}

	if originalHeadCommit == "" {
		// Without a baseline commit we cannot count commits. Fail-closed.
		return nil
	}

	stdout, _, code = runGit(ctx, worktreePath, "rev-list", "--count", originalHeadCommit+"..HEAD")
	if code != 0 {
		return nil // fail-closed
	}
	commits, err := strconv.Atoi(strings.TrimSpace(stdout))
	if err != nil {
		return nil
	}

	return &ChangeSummary{
		ChangedFiles: changedFiles,
		Commits:      commits,
	}
}
