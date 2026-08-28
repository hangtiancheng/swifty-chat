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
	"io"
	"os"
	"path/filepath"
	"strings"
)

// performPostCreationSetup propagates settings, hooks, symlinks, and gitignored files from the main
// repo into a newly created worktree.
func performPostCreationSetup(ctx context.Context, repoRoot, worktreePath string) {
	// A. Copy settings.local.json.
	copySettingsLocal(repoRoot, worktreePath)

	// B. Configure git hooks path.
	configureHooksPath(ctx, repoRoot, worktreePath)

	// C. Symlink large directories (opt-in via config).
	symlinkDirectories(repoRoot, worktreePath, getSymlinkDirectories())

	// D. Copy gitignored files from .worktreeinclude.
	CopyWorktreeIncludeFiles(ctx, repoRoot, worktreePath)
}

// copySettingsLocal copies .swiftx/settings.local.json from the main repo to the worktree. This
// propagates local settings (which may contain secrets).
func copySettingsLocal(repoRoot, worktreePath string) {
	relPath := filepath.Join(".swiftx", "settings.local.json")
	src := filepath.Join(repoRoot, relPath)
	dst := filepath.Join(worktreePath, relPath)

	srcData, err := os.ReadFile(src)
	if err != nil {
		return // ENOENT is fine — no local settings to copy
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(dst, srcData, 0o644)
}

// configureHooksPath sets core.hooksPath in the worktree so git hooks from the main repo are
// shared. Prioritizes .husky/ over .git/hooks/.
func configureHooksPath(ctx context.Context, repoRoot, worktreePath string) {
	candidates := []string{
		filepath.Join(repoRoot, ".husky"),
		filepath.Join(repoRoot, ".git", "hooks"),
	}
	var hooksPath string
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err == nil && info.IsDir() {
			hooksPath = c
			break
		}
	}
	if hooksPath == "" {
		return
	}
	_, _, code := runGit(ctx, worktreePath, "config", "core.hooksPath", hooksPath)
	if code != 0 {
		// best-effort — don't fail the whole setup.
		return
	}
}

// symlinkDirectories creates symlinks from repoRoot dirs into worktreePath to avoid disk bloat
// (e.g. node_modules, vendor).
func symlinkDirectories(repoRoot, worktreePath string, dirs []string) {
	for _, dir := range dirs {
		if strings.Contains(dir, "..") {
			continue // path traversal guard
		}
		src := filepath.Join(repoRoot, dir)
		dst := filepath.Join(worktreePath, dir)
		// symlink is best-effort: source may not exist, dest may already exist.
		_ = os.Symlink(src, dst)
	}
}

// getSymlinkDirectories returns the configured list of directories to symlink. Configured via
// settings.worktree.symlinkDirectories. We read from config if available; empty by default.
func getSymlinkDirectories() []string {
	return worktreeConfig.SymlinkDirectories
}

// CopyWorktreeIncludeFiles copies gitignored files specified in .worktreeinclude from the base repo
// to the worktree. Uses gitignore-syntax patterns.
func CopyWorktreeIncludeFiles(ctx context.Context, repoRoot, worktreePath string) ([]string, error) {
	includeFile := filepath.Join(repoRoot, ".worktreeinclude")
	data, err := os.ReadFile(includeFile)
	if err != nil {
		return nil, nil // no .worktreeinclude → nothing to copy
	}

	var patterns []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if len(patterns) == 0 {
		return nil, nil
	}

	// List gitignored files using git ls-files.
	stdout, _, code := runGit(ctx, repoRoot,
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory")
	if code != 0 || strings.TrimSpace(stdout) == "" {
		return nil, nil
	}

	entries := strings.Split(strings.TrimSpace(stdout), "\n")

	// Simple pattern matching: for each gitignored file, check if any .worktreeinclude pattern matches
	// it. We use filepath.Match for basic glob matching and prefix matching for directory patterns.
	var toCopy []string
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		// Skip collapsed directories (trailing /).
		if strings.HasSuffix(entry, "/") {
			continue
		}
		if matchesWorktreeInclude(entry, patterns) {
			toCopy = append(toCopy, entry)
		}
	}

	var copied []string
	for _, rel := range toCopy {
		src := filepath.Join(repoRoot, rel)
		dst := filepath.Join(worktreePath, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := copyFileContents(src, dst); err != nil {
			continue
		}
		copied = append(copied, rel)
	}
	return copied, nil
}

// matchesWorktreeInclude checks whether a file path matches any of the .worktreeinclude patterns.
// Supports exact match, basename match, and basic glob patterns.
func matchesWorktreeInclude(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, p := range patterns {
		p = strings.TrimPrefix(p, "/")
		// Exact match.
		if p == path || p == base {
			return true
		}
		// Glob match against full path.
		if matched, _ := filepath.Match(p, path); matched {
			return true
		}
		// Glob match against basename.
		if matched, _ := filepath.Match(p, base); matched {
			return true
		}
		// Prefix match for directory patterns.
		if strings.HasSuffix(p, "/") && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// WorktreeConfig holds worktree-related configuration. Populated from config.yaml or defaults.
var worktreeConfig = struct {
	SymlinkDirectories   []string
	StaleCleanupInterval int // seconds; 0 = disabled
	StaleCutoffHours     int // hours; default 720 (30 days)
}{
	StaleCutoffHours: 720,
}

// SetWorktreeConfig allows the TUI/CLI startup to inject config values.
func SetWorktreeConfig(symlinkDirs []string, cleanupIntervalSec, cutoffHours int) {
	worktreeConfig.SymlinkDirectories = symlinkDirs
	worktreeConfig.StaleCleanupInterval = cleanupIntervalSec
	if cutoffHours > 0 {
		worktreeConfig.StaleCutoffHours = cutoffHours
	}
}

// GetStaleCutoffHours returns the configured cutoff in hours.
func GetStaleCutoffHours() int {
	return worktreeConfig.StaleCutoffHours
}

// GetStaleCleanupInterval returns the configured cleanup interval in seconds.
func GetStaleCleanupInterval() int {
	return worktreeConfig.StaleCleanupInterval
}

// FindCanonicalGitRoot resolves through worktrees to find the main repo root. When called from
// inside a worktree, follows the .git pointer to commondir.
func FindCanonicalGitRoot(startDir string) string {
	gitDir, err := ResolveGitDir(startDir)
	if err != nil || gitDir == "" {
		return ""
	}
	// If gitDir contains a commondir pointer, follow it to the main repo.
	commonDir, err := GetCommonDir(gitDir)
	if err != nil || commonDir == "" {
		// gitDir is the main .git dir; repo root is its parent.
		return filepath.Dir(gitDir)
	}
	// commonDir points to the main repo's .git; repo root is its parent.
	return filepath.Dir(commonDir)
}
