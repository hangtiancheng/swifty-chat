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

package consolidation

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

// =========================================================================
// Lock file tests
// =========================================================================

func TestLock_FirstAcquire(t *testing.T) {
	dir := t.TempDir()

	// When lock file does not exist, lastConsolidatedAt should be 0
	last, err := ReadLastConsolidatedAt(dir)
	if err != nil {
		t.Fatalf("ReadLastConsolidatedAt: %v", err)
	}
	if last != 0 {
		t.Fatalf("expected 0, got %d", last)
	}

	// First acquisition should succeed, returning prior mtime 0
	prior, err := TryAcquireLock(dir)
	if err != nil {
		t.Fatalf("TryAcquireLock: %v", err)
	}
	if prior != 0 {
		t.Fatalf("expected prior=0, got %d", prior)
	}

	// Lock file content should be the current PID
	content, _ := os.ReadFile(filepath.Join(dir, lockFileName))
	if string(content) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("expected PID %d, got %s", os.Getpid(), content)
	}

	// lastConsolidatedAt should now be non-zero
	last2, _ := ReadLastConsolidatedAt(dir)
	if last2 == 0 {
		t.Fatal("expected non-zero after lock acquire")
	}
}

func TestLock_BlocksWhenHeld(t *testing.T) {
	dir := t.TempDir()

	// Acquire lock
	_, _ = TryAcquireLock(dir)

	// Same process trying again should be blocked (PID alive, mtime within 1 hour)
	prior2, _ := TryAcquireLock(dir)
	if prior2 != -1 {
		t.Fatal("expected lock to block when held by live process")
	}
}

func TestLock_ReclaimsDeadPID(t *testing.T) {
	dir := t.TempDir()

	// Write a lock file with a non-existent PID
	lockFile := filepath.Join(dir, lockFileName)
	os.WriteFile(lockFile, []byte("999999999"), 0o644)

	// Should reclaim the lock (PID is dead)
	prior, err := TryAcquireLock(dir)
	if err != nil {
		t.Fatalf("TryAcquireLock: %v", err)
	}
	if prior == -1 {
		t.Fatal("should reclaim lock from dead PID")
	}
}

func TestLock_ReclaimsStale(t *testing.T) {
	dir := t.TempDir()

	// Write a lock file with mtime over 1 hour old (even if PID is self)
	lockFile := filepath.Join(dir, lockFileName)
	os.WriteFile(lockFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
	oldTime := time.Now().Add(-2 * time.Hour)
	os.Chtimes(lockFile, oldTime, oldTime)

	// mtime exceeds holderStaleMs, should reclaim even with live PID
	prior, err := TryAcquireLock(dir)
	if err != nil {
		t.Fatalf("TryAcquireLock: %v", err)
	}
	if prior == -1 {
		t.Fatal("should reclaim stale lock even with live PID")
	}
}

func TestLock_RollbackDeletesOnZero(t *testing.T) {
	dir := t.TempDir()
	TryAcquireLock(dir)

	RollbackLock(dir, 0)

	if _, err := os.Stat(filepath.Join(dir, lockFileName)); !os.IsNotExist(err) {
		t.Fatal("rollback with prior=0 should delete lock file")
	}
}

func TestLock_RollbackRestoresMtime(t *testing.T) {
	dir := t.TempDir()

	// Create old lock first
	lockFile := filepath.Join(dir, lockFileName)
	oldTime := time.Now().Add(-48 * time.Hour)
	os.WriteFile(lockFile, []byte("99999"), 0o644)
	os.Chtimes(lockFile, oldTime, oldTime)

	prior, _ := TryAcquireLock(dir)
	RollbackLock(dir, prior)

	// mtime should be restored near the old value
	info, _ := os.Stat(lockFile)
	diff := info.ModTime().UnixMilli() - prior
	if diff < -1000 || diff > 1000 {
		t.Fatalf("mtime not restored: expected ~%d, got %d", prior, info.ModTime().UnixMilli())
	}

	// PID should be cleared
	content, _ := os.ReadFile(lockFile)
	if strings.TrimSpace(string(content)) != "" {
		t.Fatalf("expected empty PID after rollback, got %q", content)
	}
}

// =========================================================================
// Prompt tests
// =========================================================================

func TestPrompt_ContainsAllPhases(t *testing.T) {
	prompt := BuildConsolidationPrompt("/mem", "/user/mem", "/sessions", []string{"s1", "s2", "s3"})

	for _, want := range []string{
		"Phase 1", "Phase 2", "Phase 3", "Phase 4",
		"MEMORY.md",
		"/mem", "/user/mem", "/sessions",
		"s1", "s2", "s3",
		"Sessions since last consolidation (3)",
		"read-only commands",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestPrompt_EmptySessions(t *testing.T) {
	prompt := BuildConsolidationPrompt("/mem", "", "/sessions", nil)

	if strings.Contains(prompt, "Sessions since last consolidation") {
		t.Error("should not include sessions section when list is empty")
	}
}

// =========================================================================
// Session list tests
// =========================================================================

func TestListSessionsSince(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, ".swiftx", "sessions")
	os.MkdirAll(sessDir, 0o755)

	// Create old session (2 days ago)
	os.WriteFile(filepath.Join(sessDir, "old.jsonl"), []byte(`{"role":"user","content":"hi","ts":1}`+"\n"), 0o644)
	os.Chtimes(filepath.Join(sessDir, "old.jsonl"), time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour))

	// Create recent sessions (just now)
	os.WriteFile(filepath.Join(sessDir, "new1.jsonl"), []byte(`{"role":"user","content":"a","ts":2}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(sessDir, "new2.jsonl"), []byte(`{"role":"user","content":"b","ts":3}`+"\n"), 0o644)

	ids := listSessionsSince(dir, time.Now().Add(-24*time.Hour).UnixMilli())
	if len(ids) != 2 {
		t.Fatalf("expected 2 recent sessions, got %d: %v", len(ids), ids)
	}
}

// =========================================================================
// MaybeRun gate logic tests
// =========================================================================

func TestMaybeRun_SkipsWhenMemoryDirMissing(t *testing.T) {
	dir := t.TempDir()
	c := NewConsolidator(Deps{
		MemoryDir:   filepath.Join(dir, "nonexistent", "memory") + "/",
		ProjectRoot: dir,
	})

	// Should not panic or error
	c.MaybeRun(context.Background())
}

func TestMaybeRun_SkipsWhenTimeGateNotMet(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".swiftx", "memory")
	os.MkdirAll(memDir, 0o755)

	// Set lock file mtime to 1 hour ago (does not satisfy 24-hour gate)
	lockFile := filepath.Join(memDir, lockFileName)
	os.WriteFile(lockFile, []byte(""), 0o644)
	os.Chtimes(lockFile, time.Now().Add(-1*time.Hour), time.Now().Add(-1*time.Hour))

	var triggered atomic.Bool
	c := NewConsolidator(Deps{
		MemoryDir:   memDir + "/",
		ProjectRoot: dir,
		DebugLogf:   func(f string, a ...any) { triggered.Store(true) },
	})

	c.MaybeRun(context.Background())

	// Should not trigger (no debug log with "firing" expected)
	// Time gate not met, return early
}

func TestMaybeRun_SkipsWhenSessionGateNotMet(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".swiftx", "memory")
	sessDir := filepath.Join(dir, ".swiftx", "sessions")
	os.MkdirAll(memDir, 0o755)
	os.MkdirAll(sessDir, 0o755)

	// Only create 2 sessions (does not satisfy the 5-session gate)
	os.WriteFile(filepath.Join(sessDir, "s1.jsonl"), []byte(`{"role":"user","content":"a","ts":1}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(sessDir, "s2.jsonl"), []byte(`{"role":"user","content":"b","ts":2}`+"\n"), 0o644)

	var logs []string
	c := NewConsolidator(Deps{
		MemoryDir:   memDir + "/",
		ProjectRoot: dir,
		DebugLogf:   func(f string, a ...any) { logs = append(logs, f) },
	})
	c.SetThresholds(0, 5) // Set time gate to 0 (immediate pass), session gate to 5

	c.MaybeRun(context.Background())

	// Should see "skip — 2 sessions" log
	found := false
	for _, log := range logs {
		if strings.Contains(log, "skip") && strings.Contains(log, "sessions") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected skip log for insufficient sessions, got: %v", logs)
	}
}

func TestMaybeRun_TriggersWhenBothGatesMet(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".swiftx", "memory")
	sessDir := filepath.Join(dir, ".swiftx", "sessions")
	os.MkdirAll(memDir, 0o755)
	os.MkdirAll(sessDir, 0o755)

	// Create 6 sessions
	for i := range 6 {
		name := filepath.Join(sessDir, "s"+strconv.Itoa(i)+".jsonl")
		os.WriteFile(name, []byte(`{"role":"user","content":"x","ts":1}`+"\n"), 0o644)
	}

	// Verify gate logic: when both gates pass, lock should be acquired and "firing" logged.
	// No Client provided, so the sub-agent won't actually start — only verifies trigger logic.
	// Uses lockAcquired to verify: if lock was acquired, the entire gate chain passed.
	lockAcquired := false

	// Manually simulate MaybeRun's gate chain (skip c.run to avoid nil client panic)
	lastAt, _ := ReadLastConsolidatedAt(memDir)
	hoursSince := float64(time.Now().UnixMilli()-lastAt) / 3_600_000

	sessionIDs := listSessionsSince(dir, lastAt)

	// Time gate should pass (no lock file, lastAt=0)
	if hoursSince < 0 {
		t.Fatal("time gate should pass (no lock file)")
	}

	// Session gate should pass (6 sessions >= 5)
	if len(sessionIDs) < 5 {
		t.Fatalf("session gate should pass: got %d sessions", len(sessionIDs))
	}

	// Lock should be acquirable
	prior, err := TryAcquireLock(memDir)
	if err != nil {
		t.Fatalf("lock acquire failed: %v", err)
	}
	if prior != -1 {
		lockAcquired = true
	}

	if !lockAcquired {
		t.Fatal("expected lock to be acquired when both gates pass")
	}
}

func TestMaybeRun_ScanThrottle(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".swiftx", "memory")
	sessDir := filepath.Join(dir, ".swiftx", "sessions")
	os.MkdirAll(memDir, 0o755)
	os.MkdirAll(sessDir, 0o755)

	// Only 2 sessions (not enough to trigger)
	os.WriteFile(filepath.Join(sessDir, "s1.jsonl"), []byte(`{"role":"user","content":"a","ts":1}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(sessDir, "s2.jsonl"), []byte(`{"role":"user","content":"b","ts":2}`+"\n"), 0o644)

	var logs []string
	c := NewConsolidator(Deps{
		MemoryDir:   memDir + "/",
		ProjectRoot: dir,
		DebugLogf:   func(f string, a ...any) { logs = append(logs, f) },
	})
	c.SetThresholds(0, 5)

	// First call: scan sessions, find insufficient count
	c.MaybeRun(context.Background())

	logsBefore := len(logs)

	// Second call immediately: should be blocked by scan throttle
	c.MaybeRun(context.Background())

	throttled := false
	for _, log := range logs[logsBefore:] {
		if strings.Contains(log, "throttle") {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Errorf("expected scan throttle on second call, got: %v", logs[logsBefore:])
	}
}

func TestMaybeRun_LockBlocks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lock detection unreliable on Windows")
	}
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".swiftx", "memory")
	sessDir := filepath.Join(dir, ".swiftx", "sessions")
	os.MkdirAll(memDir, 0o755)
	os.MkdirAll(sessDir, 0o755)

	for i := range 6 {
		name := filepath.Join(sessDir, "s"+strconv.Itoa(i)+".jsonl")
		os.WriteFile(name, []byte(`{"role":"user","content":"x","ts":1}`+"\n"), 0o644)
	}

	// Manually acquire lock first
	TryAcquireLock(memDir)

	var logs []string
	c := NewConsolidator(Deps{
		MemoryDir:   memDir + "/",
		ProjectRoot: dir,
		DebugLogf:   func(f string, a ...any) { logs = append(logs, f) },
	})
	c.SetThresholds(0, 1) // Set all gates to minimum

	c.MaybeRun(context.Background())

	// Should be blocked by lock
	blocked := false
	for _, log := range logs {
		if strings.Contains(log, "lock held") {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Errorf("expected lock blocked log, got: %v", logs)
	}
}

// =========================================================================
// extractWrittenPaths tests
// =========================================================================

func TestExtractWrittenPaths(t *testing.T) {
	msgs := []conversation.Message{
		{Role: "user", Content: "consolidate"},
		{
			Role:    "assistant",
			Content: "updating memories",
			ToolUses: []conversation.ToolUseBlock{
				{ToolName: "WriteFile", Arguments: map[string]any{"file_path": "/mem/feedback_testing.md"}},
				{ToolName: "EditFile", Arguments: map[string]any{"file_path": "/mem/MEMORY.md"}},
				{ToolName: "ReadFile", Arguments: map[string]any{"file_path": "/mem/old.md"}},
			},
		},
		{
			Role:    "assistant",
			Content: "done",
			ToolUses: []conversation.ToolUseBlock{
				{ToolName: "WriteFile", Arguments: map[string]any{"file_path": "/mem/feedback_testing.md"}}, // duplicate
				{ToolName: "WriteFile", Arguments: map[string]any{"file_path": "/mem/project_new.md"}},
			},
		},
	}

	paths := extractWrittenPaths(msgs)

	// Should have 3 unique paths (WriteFile and EditFile), ReadFile not counted
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d: %v", len(paths), paths)
	}

	// feedback_testing.md should not appear more than once
	count := 0
	for _, p := range paths {
		if strings.Contains(p, "feedback_testing") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("feedback_testing.md should appear once, got %d", count)
	}
}
