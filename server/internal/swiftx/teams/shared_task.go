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

package teams

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
)

// SharedTask is a single task on the team shared task board, with dependency
// relations (Blocks / BlockedBy) and ownership (Assignee).
type SharedTask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // pending | in_progress | completed | blocked
	Assignee    string   `json:"assignee"`
	Blocks      []string `json:"blocks"`
	BlockedBy   []string `json:"blocked_by"`
	CreatedBy   string   `json:"created_by"`
}

// storeData is the overall structure of tasks.json: the next available ID plus
// the task list.
type storeData struct {
	NextID int          `json:"next_id"`
	Tasks  []SharedTask `json:"tasks"`
}

// SharedTaskStore persists to a JSON file (tasks.json) for all members of the
// same team to read and write. The file is reloaded before every read so that
// teammates in other processes always see the latest data.
type SharedTaskStore struct {
	mu     sync.Mutex
	path   string
	nextID int
	tasks  []SharedTask
}

// NewSharedTaskStore opens (or initializes) the shared task store at the given
// path.
func NewSharedTaskStore(path string) *SharedTaskStore {
	s := &SharedTaskStore{path: path, nextID: 1}
	s.load()
	return s
}

// load re-reads the task list from disk; it stays empty if the file does not
// exist. The caller must hold the lock.
func (s *SharedTaskStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var sd storeData
	if err := json.Unmarshal(data, &sd); err != nil {
		return
	}
	s.tasks = sd.Tasks
	if sd.NextID > 0 {
		s.nextID = sd.NextID
	}
}

// save writes the current task list back to disk. The caller must hold the
// lock.
func (s *SharedTaskStore) save() {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	sd := storeData{NextID: s.nextID, Tasks: s.tasks}
	if s.tasks == nil {
		sd.Tasks = []SharedTask{}
	}
	out, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, out, 0o644)
}

// Create creates a shared task and returns it.
func (s *SharedTaskStore) Create(title, description, assignee string, blocks, blockedBy []string, createdBy string) SharedTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	if blocks == nil {
		blocks = []string{}
	}
	if blockedBy == nil {
		blockedBy = []string{}
	}
	task := SharedTask{
		ID:          strconv.Itoa(s.nextID),
		Title:       title,
		Description: description,
		Status:      "pending",
		Assignee:    assignee,
		Blocks:      blocks,
		BlockedBy:   blockedBy,
		CreatedBy:   createdBy,
	}
	s.nextID++
	s.tasks = append(s.tasks, task)
	s.save()
	return task
}

// Get returns the task with the given ID, reloading first to get the latest
// data. It returns nil if the task is not found.
func (s *SharedTaskStore) Get(id string) *SharedTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	for i := range s.tasks {
		if s.tasks[i].ID == id {
			t := s.tasks[i]
			return &t
		}
	}
	return nil
}

// ListTasks lists tasks, optionally filtered by status and assignee.
func (s *SharedTaskStore) ListTasks(status, assignee string) []SharedTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	var result []SharedTask
	for _, t := range s.tasks {
		if status != "" && t.Status != status {
			continue
		}
		if assignee != "" && t.Assignee != assignee {
			continue
		}
		result = append(result, t)
	}
	return result
}

// TaskUpdate describes a single update; a nil pointer leaves the corresponding
// field unchanged.
type TaskUpdate struct {
	Status       *string
	Assignee     *string
	Description  *string
	AddBlocks    []string
	AddBlockedBy []string
}

// Update modifies a task according to TaskUpdate; AddBlocks / AddBlockedBy
// append dependencies (deduplicated). It returns nil if the task does not
// exist.
func (s *SharedTaskStore) Update(id string, upd TaskUpdate) *SharedTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	for i := range s.tasks {
		if s.tasks[i].ID != id {
			continue
		}
		t := &s.tasks[i]
		if upd.Status != nil {
			t.Status = *upd.Status
		}
		if upd.Assignee != nil {
			t.Assignee = *upd.Assignee
		}
		if upd.Description != nil {
			t.Description = *upd.Description
		}
		t.Blocks = appendUnique(t.Blocks, upd.AddBlocks)
		t.BlockedBy = appendUnique(t.BlockedBy, upd.AddBlockedBy)
		s.save()
		updated := *t
		return &updated
	}
	return nil
}

// InitEmpty clears the task store and persists it, used when initializing a
// newly created team.
func (s *SharedTaskStore) InitEmpty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = []SharedTask{}
	s.nextID = 1
	s.save()
}

// appendUnique appends elements from add that are not yet in base and returns
// the new slice.
func appendUnique(base, add []string) []string {
	for _, v := range add {
		found := slices.Contains(base, v)
		if !found {
			base = append(base, v)
		}
	}
	if base == nil {
		base = []string{}
	}
	return base
}
