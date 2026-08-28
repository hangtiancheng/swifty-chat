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

// Package worktree filesystem helpers: read git state without spawning git subprocesses.
//
// Covers: resolving .git directories (including worktrees/submodules), parsing HEAD, resolving refs
// via loose files and packed-refs.
//
// Correctness notes (verified against git source):
// HEAD: `ref: refs/heads/<branch>\n` or raw SHA (refs/files-backend.c)
// Packed-refs: `<sha> <refname>\n`, skip `#` and `^` lines (packed-backend.c)
// git file (worktree): `gitdir: <path>\n` with optional relative path (setup.c)
package worktree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// safeRefName allows ASCII alphanumerics, '/', '.', '_', '+', '-', '@'. Used to validate ref/branch
// names read from .git/ so a tampered HEAD or ref file can't inject path traversal, argument
// prefixes, or shell metacharacters.
var safeRefName = regexp.MustCompile(`^[a-zA-Z0-9/._+@-]+$`)

// IsSafeRefName validates that a ref/branch name is safe to use in path joins, as git positional
// arguments, and when interpolated into shell commands.
func IsSafeRefName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	// Reject single-dot and empty path components.
	for seg := range strings.SplitSeq(name, "/") {
		if seg == "." || seg == "" {
			return false
		}
	}
	return safeRefName.MatchString(name)
}

var sha1Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// IsValidGitSha reports whether s is a full-length SHA-1 (40 hex) or SHA-256 (64 hex) git object
// id. Git never writes abbreviated SHAs to HEAD or ref files.
func IsValidGitSha(s string) bool {
	return sha1Pattern.MatchString(s) || sha256Pattern.MatchString(s)
}

// ResolveGitDir resolves the actual .git directory for a repo rooted at root. Handles
// worktrees/submodules where .git is a file containing `gitdir: <path>`. Returns ("", nil) when
// root has no .git entry (not a repo) — the caller treats empty as "not a git repo". Errors are
// reserved for IO failures the caller cares about (here: only filesystem errors other than ENOENT).
//
// minus the memoization (Go callers cache at higher layers).
func ResolveGitDir(root string) (string, error) {
	gitPath := filepath.Join(root, ".git")
	st, err := os.Stat(gitPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if !st.IsDir() {
		// Worktree or submodule: .git is a file with `gitdir: <path>`. Git strips trailing whitespace via
		// strbuf_rtrim (setup.c read_gitfile_gently); strings.TrimSpace is equivalent.
		raw, err := os.ReadFile(gitPath)
		if err != nil {
			return "", err
		}
		content := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(content, "gitdir:") {
			return "", nil
		}
		rel := strings.TrimSpace(strings.TrimPrefix(content, "gitdir:"))
		// resolve relative to the root (where the .git pointer file lives).
		if filepath.IsAbs(rel) {
			return rel, nil
		}
		return filepath.Clean(filepath.Join(root, rel)), nil
	}
	return gitPath, nil
}

// GetCommonDir reads the `commondir` file inside a worktree's gitDir to find the shared git
// directory. In a worktree, this points to the main repo's .git dir. Returns ("", nil) if no
// commondir file exists (regular repo).
func GetCommonDir(gitDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	content := strings.TrimSpace(string(raw))
	if filepath.IsAbs(content) {
		return content, nil
	}
	return filepath.Clean(filepath.Join(gitDir, content)), nil
}

// gitHead is the parsed result of <gitDir>/HEAD.
type gitHead struct {
	// branch is non-empty when HEAD is on a branch.
	branch string
	// sha is non-empty when HEAD is detached (raw SHA) or when an unusual symref has been resolved.
	sha string
}

// readGitHead parses <gitDir>/HEAD to determine current branch or detached SHA. Returns (nil, nil)
// when HEAD doesn't exist or is malformed — callers treat that as "not a worktree" / "not a repo".
// IO errors other than ENOENT propagate.
//
// HEAD format (per refs/files-backend.c):
// `ref: refs/heads/<branch>\n` — on a branch
// `ref: <other-ref>\n` — unusual symref (e.g. during bisect)
// `<hex-sha>\n` — detached HEAD (e.g. during rebase)
func readGitHead(gitDir string) (*gitHead, error) {
	raw, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	content := strings.TrimSpace(string(raw))
	if after, ok := strings.CutPrefix(content, "ref:"); ok {
		ref := strings.TrimSpace(after)
		if after, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			name := after
			if !IsSafeRefName(name) {
				return nil, nil
			}
			return &gitHead{branch: name}, nil
		}
		// Unusual symref (not a local branch) — resolve to SHA.
		if !IsSafeRefName(ref) {
			return nil, nil
		}
		sha, err := ResolveRef(gitDir, ref)
		if err != nil {
			return nil, err
		}
		return &gitHead{sha: sha}, nil
	}
	// Raw SHA (detached HEAD). Validate so a tampered HEAD can't flow shell metacharacters into
	// downstream contexts.
	if !IsValidGitSha(content) {
		return nil, nil
	}
	return &gitHead{sha: content}, nil
}

// ResolveRef resolves a git ref (e.g. `refs/heads/main`) to a commit SHA. Checks loose ref files
// first, then falls back to packed-refs. Follows symrefs (e.g. `ref: refs/remotes/origin/main`).
//
// For worktrees, refs live in the common gitdir (pointed to by the `commondir` file), not the
// worktree-specific gitdir. We check the worktree gitdir first, then fall back to the common dir.
func ResolveRef(gitDir, ref string) (string, error) {
	sha, err := resolveRefInDir(gitDir, ref)
	if err != nil {
		return "", err
	}
	if sha != "" {
		return sha, nil
	}
	commonDir, err := GetCommonDir(gitDir)
	if err != nil {
		return "", err
	}
	if commonDir != "" && commonDir != gitDir {
		return resolveRefInDir(commonDir, ref)
	}
	return "", nil
}

// resolveRefInDir resolves ref within a single git directory (no commonDir fallback).
func resolveRefInDir(dir, ref string) (string, error) {
	// Try loose ref file first.
	raw, err := os.ReadFile(filepath.Join(dir, ref))
	if err == nil {
		content := strings.TrimSpace(string(raw))
		if after, ok := strings.CutPrefix(content, "ref:"); ok {
			target := strings.TrimSpace(after)
			if !IsSafeRefName(target) {
				return "", nil
			}
			// Recurse to follow the symref chain. Pass `dir` (not gitDir) so resolveRef's commonDir fallback
			// applies from the same starting point.
			return ResolveRef(dir, target)
		}
		if !IsValidGitSha(content) {
			return "", nil
		}
		return content, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	// Fall back to packed-refs.
	packed, err := os.ReadFile(filepath.Join(dir, "packed-refs"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	for line := range strings.SplitSeq(string(packed), "\n") {
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		before, after, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if after == ref {
			sha := before
			if !IsValidGitSha(sha) {
				return "", nil
			}
			return sha, nil
		}
	}
	return "", nil
}

// ReadRawSymref reads a raw symref file and extracts the branch name after a known prefix. Returns
// ("", nil) if the ref doesn't exist, isn't a symref, or doesn't match the prefix. Checks loose
// file only — packed-refs doesn't store symrefs.
func ReadRawSymref(gitDir, refPath, branchPrefix string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(gitDir, refPath))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	content := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(content, "ref:") {
		return "", nil
	}
	target := strings.TrimSpace(strings.TrimPrefix(content, "ref:"))
	if !strings.HasPrefix(target, branchPrefix) {
		return "", nil
	}
	name := strings.TrimPrefix(target, branchPrefix)
	if !IsSafeRefName(name) {
		return "", nil
	}
	return name, nil
}

// GetDefaultBranch determines the repo's default branch by reading refs/remotes/origin/HEAD (a
// symref) from the common gitdir, falling back to trying `main` then `master`, and finally
// returning "main".
//
// Reads purely from the filesystem — no git subprocess, no network.
func GetDefaultBranch(repoRoot string) (string, error) {
	gitDir, err := ResolveGitDir(repoRoot)
	if err != nil {
		return "main", err
	}
	if gitDir == "" {
		return "main", nil
	}
	// refs/remotes/ lives in commonDir, not the per-worktree gitDir.
	commonDir, err := GetCommonDir(gitDir)
	if err != nil {
		return "main", err
	}
	if commonDir == "" {
		commonDir = gitDir
	}
	branch, err := ReadRawSymref(commonDir, "refs/remotes/origin/HEAD", "refs/remotes/origin/")
	if err != nil {
		return "main", err
	}
	if branch != "" {
		return branch, nil
	}
	for _, candidate := range []string{"main", "master"} {
		sha, err := ResolveRef(commonDir, "refs/remotes/origin/"+candidate)
		if err != nil {
			return "main", err
		}
		if sha != "" {
			return candidate, nil
		}
	}
	return "main", nil
}

// GetCurrentBranch reads <repoRoot>/.git/HEAD and returns the current branch name, or "" when HEAD
// is detached. Pure filesystem read; (but distinguishes detached HEAD via empty string instead of
// the sentinel "HEAD").
func GetCurrentBranch(repoRoot string) (string, error) {
	gitDir, err := ResolveGitDir(repoRoot)
	if err != nil || gitDir == "" {
		return "", err
	}
	head, err := readGitHead(gitDir)
	if err != nil || head == nil {
		return "", err
	}
	return head.branch, nil
}

// ReadWorktreeHeadSha reads the HEAD SHA for a git worktree directory (not the main repo). Unlike
// ResolveGitDir+readGitHead chained, this reads `<worktreePath>/.git` directly as a `gitdir:`
// pointer file, with no upward walk. Returns ("", nil) when the worktree doesn't exist (`.git`
// pointer ENOENT) or is malformed; callers treat empty as "not a valid worktree".
//
// Target perf: ≤10ms (pure filesystem reads, no subprocess). On a 16M-object repo `git rev-parse
// HEAD` would burn ~15ms on spawn overhead alone.
func ReadWorktreeHeadSha(worktreePath string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	ptr := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(ptr, "gitdir:") {
		return "", nil
	}
	rel := strings.TrimSpace(strings.TrimPrefix(ptr, "gitdir:"))
	var gitDir string
	if filepath.IsAbs(rel) {
		gitDir = rel
	} else {
		gitDir = filepath.Clean(filepath.Join(worktreePath, rel))
	}
	head, err := readGitHead(gitDir)
	if err != nil {
		return "", err
	}
	if head == nil {
		return "", nil
	}
	if head.branch != "" {
		return ResolveRef(gitDir, "refs/heads/"+head.branch)
	}
	return head.sha, nil
}
