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

package file_history

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxSnapshots = 100

type Backup struct {
	BackupPath string    `json:"backup_path"`
	Version    int       `json:"version"`
	Time       time.Time `json:"time"`
}

type Snapshot struct {
	MessageIndex int               `json:"message_index"`
	UserText     string            `json:"user_text"`
	Backups      map[string]Backup `json:"backups"`
	Timestamp    time.Time         `json:"timestamp"`
}

type History struct {
	mu           sync.Mutex
	sessionDir   string
	trackedFiles map[string]int // filepath → current version
	snapshots    []Snapshot
}

func New(baseDir, sessionID string) *History {
	dir := filepath.Join(baseDir, ".swiftx", "file-history", sessionID)
	_ = os.MkdirAll(dir, 0o755)
	return &History{
		sessionDir:   dir,
		trackedFiles: make(map[string]int),
	}
}

func backupName(filePath string, version int) string {
	h := sha256.Sum256([]byte(filePath))
	return fmt.Sprintf("%x@v%d", h[:8], version)
}

// TrackEdit backs up the file at path before it gets modified. Call this before
// any write/edit operation. If the file doesn't exist yet (new file), no backup
// is created but the path is still tracked so Rewind can delete it.
func (h *History) TrackEdit(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	ver := h.trackedFiles[absPath]
	newVer := ver + 1

	data, err := os.ReadFile(absPath)
	if err == nil {
		bp := filepath.Join(h.sessionDir, backupName(absPath, newVer))
		_ = os.WriteFile(bp, data, 0o644)
	}
	// If file doesn't exist, we still bump the version so Rewind knows the file
	// didn't exist at this version (no backup file on disk → delete on rewind).

	h.trackedFiles[absPath] = newVer
}

// MakeSnapshot creates a checkpoint associated with the given conversation
// message index. userText is a short label for the UI.
func (h *History) MakeSnapshot(msgIndex int, userText string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	backups := make(map[string]Backup, len(h.trackedFiles))
	for path, ver := range h.trackedFiles {
		bp := filepath.Join(h.sessionDir, backupName(path, ver))
		if _, err := os.Stat(bp); err != nil {
			if data, readErr := os.ReadFile(path); readErr == nil {
				_ = os.WriteFile(bp, data, 0o644)
			}
		}
		backups[path] = Backup{BackupPath: bp, Version: ver, Time: time.Now()}
	}

	snap := Snapshot{
		MessageIndex: msgIndex,
		UserText:     userText,
		Backups:      backups,
		Timestamp:    time.Now(),
	}

	h.snapshots = append(h.snapshots, snap)
	if len(h.snapshots) > maxSnapshots {
		h.snapshots = h.snapshots[len(h.snapshots)-maxSnapshots:]
	}
}

// GetSnapshots returns a copy of all snapshots for UI display.
func (h *History) GetSnapshots() []Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Snapshot, len(h.snapshots))
	copy(out, h.snapshots)
	return out
}

// Rewind restores files to the state captured in the snapshot at the given
// index. Returns the list of files that were actually changed.
func (h *History) Rewind(snapshotIndex int) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if snapshotIndex < 0 || snapshotIndex >= len(h.snapshots) {
		return nil, fmt.Errorf("invalid snapshot index %d", snapshotIndex)
	}

	target := h.snapshots[snapshotIndex]
	var changed []string

	for path, backup := range target.Backups {
		backupData, err := os.ReadFile(backup.BackupPath)
		if err != nil {
			// Backup file missing → file didn't exist at that point; delete it
			if _, statErr := os.Stat(path); statErr == nil {
				_ = os.Remove(path)
				changed = append(changed, path)
			}
			continue
		}

		currentData, _ := os.ReadFile(path)
		if string(currentData) != string(backupData) {
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			if writeErr := os.WriteFile(path, backupData, 0o644); writeErr == nil {
				changed = append(changed, path)
			}
		}
	}

	// Truncate snapshots: remove everything after the target
	h.snapshots = h.snapshots[:snapshotIndex+1]

	// Reset tracked file versions to the snapshot's versions
	for path, backup := range target.Backups {
		h.trackedFiles[path] = backup.Version
	}

	return changed, nil
}

// HasSnapshots returns true if there's at least one snapshot to rewind to.
func (h *History) HasSnapshots() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.snapshots) > 0
}

// Save persists snapshot metadata to disk.
func (h *History) Save() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	data, err := json.MarshalIndent(h.snapshots, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(h.sessionDir, "snapshots.json"), data, 0o644)
}
