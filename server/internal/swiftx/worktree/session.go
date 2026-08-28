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
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WorktreeSession tracks the state of an active worktree session.
type WorktreeSession struct {
	OriginalCwd        string `json:"original_cwd"`
	WorktreePath       string `json:"worktree_path"`
	WorktreeName       string `json:"worktree_name"`
	WorktreeBranch     string `json:"worktree_branch,omitempty"`
	OriginalBranch     string `json:"original_branch,omitempty"`
	OriginalHeadCommit string `json:"original_head_commit,omitempty"`
	SessionID          string `json:"session_id"`
	HookBased          bool   `json:"hook_based,omitempty"`
	CreationDurationMs int64  `json:"creation_duration_ms,omitempty"`
}

// Module-level singleton + mutex.
var (
	currentWorktreeSession *WorktreeSession
	sessionMu              sync.RWMutex
)

// GetCurrentWorktreeSession returns the active worktree session, or nil.
func GetCurrentWorktreeSession() *WorktreeSession {
	sessionMu.RLock()
	defer sessionMu.RUnlock()
	return currentWorktreeSession
}

// RestoreWorktreeSession restores a session on --resume. The caller must have already verified the
// directory exists and set bootstrap state.
func RestoreWorktreeSession(session *WorktreeSession) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	currentWorktreeSession = session
}

// sessionFilePath returns the path to the session persistence file.
func sessionFilePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".swiftx", "worktree_session.json")
}

// SaveWorktreeSession persists session state to disk. Pass nil to clear.
func SaveWorktreeSession(repoRoot string, session *WorktreeSession) error {
	path := sessionFilePath(repoRoot)
	if session == nil {
		_ = os.Remove(path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadWorktreeSession reads a previously persisted session from disk. Returns (nil, nil) if the
// file does not exist.
func LoadWorktreeSession(repoRoot string) (*WorktreeSession, error) {
	path := sessionFilePath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var session WorktreeSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// CreateWorktreeForSession creates or resumes a worktree and sets up the global session singleton.
func CreateWorktreeForSession(ctx context.Context, sessionID, slug, repoRoot string) (*WorktreeSession, error) {
	if err := ValidateWorktreeSlug(slug); err != nil {
		return nil, err
	}

	if repoRoot == "" {
		return nil, errorf("cannot create a worktree: not in a git repository")
	}

	originalCwd, _ := os.Getwd()
	originalBranch, _ := GetCurrentBranch(repoRoot)

	start := time.Now()
	result, err := getOrCreateWorktree(ctx, repoRoot, slug)
	if err != nil {
		return nil, err
	}

	var creationDurationMs int64
	if !result.Existed {
		performPostCreationSetup(ctx, repoRoot, result.WorktreePath)
		creationDurationMs = time.Since(start).Milliseconds()
	}

	session := &WorktreeSession{
		OriginalCwd:        originalCwd,
		WorktreePath:       result.WorktreePath,
		WorktreeName:       slug,
		WorktreeBranch:     result.WorktreeBranch,
		OriginalBranch:     originalBranch,
		OriginalHeadCommit: result.HeadCommit,
		SessionID:          sessionID,
		CreationDurationMs: creationDurationMs,
	}

	sessionMu.Lock()
	currentWorktreeSession = session
	sessionMu.Unlock()

	_ = SaveWorktreeSession(repoRoot, session)
	return session, nil
}

// KeepWorktree preserves the worktree on disk and clears session state.
func KeepWorktree(repoRoot string) error {
	sessionMu.Lock()
	session := currentWorktreeSession
	currentWorktreeSession = nil
	sessionMu.Unlock()

	if session == nil {
		return nil
	}

	if err := os.Chdir(session.OriginalCwd); err != nil {
		return err
	}

	_ = SaveWorktreeSession(repoRoot, nil)
	return nil
}

// CleanupWorktree removes the worktree and its temporary branch, then clears session state.
func CleanupWorktree(ctx context.Context, repoRoot string) error {
	sessionMu.Lock()
	session := currentWorktreeSession
	currentWorktreeSession = nil
	sessionMu.Unlock()

	if session == nil {
		return nil
	}

	if err := os.Chdir(session.OriginalCwd); err != nil {
		return err
	}

	// Remove the worktree directory via git.
	_, _, code := runGit(ctx, session.OriginalCwd,
		"worktree", "remove", "--force", session.WorktreePath)
	if code != 0 {
		// best-effort: proceed to branch cleanup.
	}

	// Delete the temporary branch.
	if session.WorktreeBranch != "" {
		// Wait for git lockfile release.
		time.Sleep(100 * time.Millisecond)
		runGit(ctx, session.OriginalCwd, "branch", "-D", session.WorktreeBranch)
	}

	_ = SaveWorktreeSession(repoRoot, nil)
	return nil
}

func errorf(format string, args ...any) error {
	return &worktreeError{msg: format}
}

type worktreeError struct {
	msg string
}

func (e *worktreeError) Error() string { return e.msg }
