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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const lockFileName = ".consolidate-lock"

// holderStaleMs is the maximum lock hold time; after this, the lock is considered expired even if the PID is still alive (prevents PID reuse)
const holderStaleMs = 60 * 60 * 1000

func lockPath(memoryDir string) string {
	return filepath.Join(memoryDir, lockFileName)
}

// ReadLastConsolidatedAt returns the timestamp of the last consolidation completion (lock file mtime).
// Returns 0 if the lock file does not exist. Cost per check: one stat call.
func ReadLastConsolidatedAt(memoryDir string) (int64, error) {
	info, err := os.Stat(lockPath(memoryDir))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.ModTime().UnixMilli(), nil
}

// TryAcquireLock attempts to acquire the consolidation lock. On success returns the prior mtime
// (for rollback on failure); on failure returns -1 (another process holds it).
//
// Acquisition flow:
//  1. Read the lock file's mtime and PID
//  2. If the file exists, mtime is within 1 hour, and the PID process is alive -> give up
//  3. Otherwise write own PID
//  4. Read back to verify; if PID is not ours -> race lost
func TryAcquireLock(memoryDir string) (priorMtime int64, err error) {
	path := lockPath(memoryDir)

	var mtimeMs int64
	var holderPid int
	var fileExists bool

	if info, statErr := os.Stat(path); statErr == nil {
		fileExists = true
		mtimeMs = info.ModTime().UnixMilli()
		if raw, readErr := os.ReadFile(path); readErr == nil {
			if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw))); parseErr == nil {
				holderPid = pid
			}
		}
	}

	if fileExists && time.Now().UnixMilli()-mtimeMs < holderStaleMs {
		if holderPid > 0 && isProcessRunning(holderPid) {
			return -1, nil
		}
	}

	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return -1, fmt.Errorf("write lock: %w", err)
	}

	verify, err := os.ReadFile(path)
	if err != nil {
		return -1, fmt.Errorf("verify lock: %w", err)
	}
	if strings.TrimSpace(string(verify)) != strconv.Itoa(os.Getpid()) {
		return -1, nil
	}

	if fileExists {
		return mtimeMs, nil
	}
	return 0, nil
}

// RollbackLock restores the lock file's mtime to pre-acquisition value, used for recovery after consolidation failure.
// When priorMtime is 0, the lock file is deleted directly.
func RollbackLock(memoryDir string, priorMtime int64) {
	path := lockPath(memoryDir)
	if priorMtime == 0 {
		os.Remove(path)
		return
	}
	// Clear PID to prevent our own PID from being mistaken as still holding
	os.WriteFile(path, []byte(""), 0o644)
	t := time.UnixMilli(priorMtime)
	os.Chtimes(path, t, t)
}

// isProcessRunning is implemented in platform-specific files (lock_unix.go / lock_windows.go)
