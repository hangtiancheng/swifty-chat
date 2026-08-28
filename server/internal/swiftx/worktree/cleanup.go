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
	"regexp"
	"strings"
	"time"
)

// ephemeralWorktreePatterns identifies throwaway worktrees that can be auto-cleaned. User-named
// worktrees (e.g. "my-feature") never match.
var ephemeralWorktreePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^agent-a[0-9a-f]{7}$`),
	regexp.MustCompile(`^wf_[0-9a-f]{8}-[0-9a-f]{3}-\d+$`),
	regexp.MustCompile(`^wf-\d+$`),
	regexp.MustCompile(`^bridge-[A-Za-z0-9_]+(-[A-Za-z0-9_]+)*$`),
	regexp.MustCompile(`^job-[a-zA-Z0-9._-]{1,55}-[0-9a-f]{8}$`),
}

func isEphemeralSlug(slug string) bool {
	for _, p := range ephemeralWorktreePatterns {
		if p.MatchString(slug) {
			return true
		}
	}
	return false
}

// CleanupStaleAgentWorktrees removes stale agent/workflow worktrees older than cutoffDate. Three-
// layer safety filter:
//
// 1. Name pattern: only ephemeral slugs 2. Age + session: skip current session and recently
// modified 3. Change check: skip if tracked changes or unpushed commits.
func CleanupStaleAgentWorktrees(ctx context.Context, cutoffDate time.Time) int {
	cwd, _ := os.Getwd()
	gitRoot := FindCanonicalGitRoot(cwd)
	if gitRoot == "" {
		return 0
	}

	dir := WorktreesDir(gitRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	currentPath := ""
	if s := GetCurrentWorktreeSession(); s != nil {
		currentPath = s.WorktreePath
	}

	removed := 0
	for _, entry := range entries {
		slug := entry.Name()

		// Layer 1: only ephemeral patterns.
		if !isEphemeralSlug(slug) {
			continue
		}

		worktreePath := WorktreePathFor(gitRoot, slug)
		if currentPath == worktreePath {
			continue
		}

		// Layer 2: age check.
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoffDate) {
			continue
		}

		// Layer 3: fail-closed change checks -uno: untracked files in a stale crashed agent worktree are
		// build artifacts; skipping the untracked scan is 5-10× faster on large repos.
		statusOut, _, statusCode := runGit(ctx, worktreePath,
			"--no-optional-locks", "status", "--porcelain", "-uno")
		if statusCode != 0 || strings.TrimSpace(statusOut) != "" {
			continue
		}

		unpushedOut, _, unpushedCode := runGit(ctx, worktreePath,
			"rev-list", "--max-count=1", "HEAD", "--not", "--remotes")
		if unpushedCode != 0 || strings.TrimSpace(unpushedOut) != "" {
			continue
		}

		if RemoveAgentWorktree(ctx, worktreePath, WorktreeBranchName(slug), gitRoot) {
			removed++
		}
	}

	if removed > 0 {
		runGit(ctx, gitRoot, "worktree", "prune")
	}
	return removed
}

// StartCleanupLoop runs periodic stale worktree cleanup in a background goroutine. Returns
// immediately; the goroutine exits when ctx is cancelled.
func StartCleanupLoop(ctx context.Context) {
	interval := GetStaleCleanupInterval()
	if interval <= 0 {
		return
	}
	cutoffHours := GetStaleCutoffHours()

	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-time.Duration(cutoffHours) * time.Hour)
				CleanupStaleAgentWorktrees(ctx, cutoff)
			}
		}
	}()
}
