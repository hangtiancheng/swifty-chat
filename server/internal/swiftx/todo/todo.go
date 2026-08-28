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

package todo

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusCompleted  TaskStatus = "completed"
)

type Task struct {
	ID          string         `json:"id"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Status      TaskStatus     `json:"status"`
	Owner       string         `json:"owner,omitempty"`
	Blocks      []string       `json:"blocks"`
	BlockedBy   []string       `json:"blockedBy"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type TaskList struct {
	mu    sync.Mutex
	store *Store
}

func NewTaskList(store *Store) *TaskList {
	return &TaskList{store: store}
}

func (tl *TaskList) Create(subject, description, activeForm string, metadata map[string]any) (*Task, error) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tasks, err := tl.store.Load()
	if err != nil {
		tasks = nil
	}

	task := &Task{
		ID:          generateID(),
		Subject:     subject,
		Description: description,
		ActiveForm:  activeForm,
		Status:      StatusPending,
		Blocks:      []string{},
		BlockedBy:   []string{},
		Metadata:    metadata,
	}

	tasks = append(tasks, task)
	if err := tl.store.Save(tasks); err != nil {
		return nil, err
	}
	return task, nil
}

func (tl *TaskList) Get(id string) (*Task, error) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tasks, err := tl.store.Load()
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, nil
}

func (tl *TaskList) List() ([]*Task, error) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tasks, err := tl.store.Load()
	if err != nil {
		return nil, err
	}

	var visible []*Task
	for _, t := range tasks {
		if t.Metadata != nil {
			if _, internal := t.Metadata["_internal"]; internal {
				continue
			}
		}
		visible = append(visible, t)
	}
	return visible, nil
}

func (tl *TaskList) Update(id string, updates map[string]any) (*Task, []string, error) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tasks, err := tl.store.Load()
	if err != nil {
		return nil, nil, err
	}

	var target *Task
	for _, t := range tasks {
		if t.ID == id {
			target = t
			break
		}
	}
	if target == nil {
		return nil, nil, nil
	}

	if status, ok := updates["status"]; ok {
		if s, ok := status.(string); ok && s == "deleted" {
			var remaining []*Task
			for _, t := range tasks {
				if t.ID != id {
					remaining = append(remaining, t)
				}
			}
			if err := tl.store.Save(remaining); err != nil {
				return nil, nil, err
			}
			return target, []string{"deleted"}, nil
		}
	}

	var changed []string

	if v, ok := updates["subject"]; ok {
		if s, ok := v.(string); ok && s != target.Subject {
			target.Subject = s
			changed = append(changed, "subject")
		}
	}
	if v, ok := updates["description"]; ok {
		if s, ok := v.(string); ok && s != target.Description {
			target.Description = s
			changed = append(changed, "description")
		}
	}
	if v, ok := updates["activeForm"]; ok {
		if s, ok := v.(string); ok && s != target.ActiveForm {
			target.ActiveForm = s
			changed = append(changed, "activeForm")
		}
	}
	if v, ok := updates["status"]; ok {
		if s, ok := v.(string); ok {
			newStatus := TaskStatus(s)
			if newStatus != target.Status {
				target.Status = newStatus
				changed = append(changed, "status")
			}
		}
	}
	if v, ok := updates["owner"]; ok {
		if s, ok := v.(string); ok && s != target.Owner {
			target.Owner = s
			changed = append(changed, "owner")
		}
	}
	if v, ok := updates["addBlocks"]; ok {
		if ids, ok := toStringSlice(v); ok && len(ids) > 0 {
			existing := make(map[string]bool)
			for _, b := range target.Blocks {
				existing[b] = true
			}
			for _, b := range ids {
				if !existing[b] {
					target.Blocks = append(target.Blocks, b)
				}
			}
			changed = append(changed, "blocks")
		}
	}
	if v, ok := updates["addBlockedBy"]; ok {
		if ids, ok := toStringSlice(v); ok && len(ids) > 0 {
			existing := make(map[string]bool)
			for _, b := range target.BlockedBy {
				existing[b] = true
			}
			for _, b := range ids {
				if !existing[b] {
					target.BlockedBy = append(target.BlockedBy, b)
				}
			}
			changed = append(changed, "blockedBy")
		}
	}
	if v, ok := updates["metadata"]; ok {
		if m, ok := v.(map[string]any); ok {
			if target.Metadata == nil {
				target.Metadata = make(map[string]any)
			}
			for k, val := range m {
				if val == nil {
					delete(target.Metadata, k)
				} else {
					target.Metadata[k] = val
				}
			}
			changed = append(changed, "metadata")
		}
	}

	if len(changed) > 0 {
		if err := tl.store.Save(tasks); err != nil {
			return nil, nil, err
		}
	}

	return target, changed, nil
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return "t" + hex.EncodeToString(b)
}

func toStringSlice(v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result, true
}
