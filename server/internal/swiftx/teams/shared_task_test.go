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
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *SharedTaskStore {
	t.Helper()
	return NewSharedTaskStore(filepath.Join(t.TempDir(), "tasks.json"))
}

//go:fix inline
func strptr(s string) *string { return new(s) }

func TestSharedTaskCreateAssignsStringIDsAndPending(t *testing.T) {
	store := newTestStore(t)
	t1 := store.Create("first", "", "", nil, nil, "lead")
	t2 := store.Create("second", "desc", "alice", nil, nil, "lead")

	if t1.ID != "1" || t2.ID != "2" {
		t.Fatalf("ids = %q,%q, want 1,2", t1.ID, t2.ID)
	}
	if t1.Status != "pending" {
		t.Fatalf("status = %q, want pending", t1.Status)
	}
	if t2.Assignee != "alice" || t2.Description != "desc" || t2.CreatedBy != "lead" {
		t.Fatalf("unexpected task2 fields: %+v", t2)
	}
}

func TestSharedTaskGetAndList(t *testing.T) {
	store := newTestStore(t)
	store.Create("a", "", "alice", nil, nil, "")
	b := store.Create("b", "", "bob", nil, nil, "")
	store.Update(b.ID, TaskUpdate{Status: new("completed")})

	if store.Get("999") != nil {
		t.Fatalf("get missing should be nil")
	}
	if len(store.ListTasks("", "")) != 2 {
		t.Fatalf("list all should be 2")
	}
	if len(store.ListTasks("completed", "")) != 1 {
		t.Fatalf("filter by status failed")
	}
	if len(store.ListTasks("", "alice")) != 1 {
		t.Fatalf("filter by assignee failed")
	}
	if len(store.ListTasks("completed", "alice")) != 0 {
		t.Fatalf("combined filter failed")
	}
}

func TestSharedTaskUpdateAndDeps(t *testing.T) {
	store := newTestStore(t)
	task := store.Create("task", "", "", nil, nil, "")
	updated := store.Update(task.ID, TaskUpdate{
		Status:       new("in_progress"),
		Assignee:     new("carol"),
		Description:  new("new desc"),
		AddBlocks:    []string{"2"},
		AddBlockedBy: []string{"3"},
	})
	if updated == nil || updated.Status != "in_progress" || updated.Assignee != "carol" {
		t.Fatalf("update result unexpected: %+v", updated)
	}
	if len(updated.Blocks) != 1 || updated.Blocks[0] != "2" {
		t.Fatalf("blocks not appended: %+v", updated.Blocks)
	}
	// Appending the same dependency again is deduplicated.
	again := store.Update(task.ID, TaskUpdate{AddBlocks: []string{"2"}})
	if len(again.Blocks) != 1 {
		t.Fatalf("dedup failed: %+v", again.Blocks)
	}
	if store.Update("nope", TaskUpdate{Status: new("completed")}) != nil {
		t.Fatalf("update missing should be nil")
	}
}

func TestSharedTaskPersistenceAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store1 := NewSharedTaskStore(path)
	store1.Create("persisted", "", "", nil, nil, "lead")

	// A second instance (simulating a teammate process) reads the same file.
	store2 := NewSharedTaskStore(path)
	if len(store2.ListTasks("", "")) != 1 {
		t.Fatalf("store2 should see 1 task")
	}
	// After store2 writes, store1 reloads before reading.
	store2.Create("from-teammate", "", "", nil, nil, "bob")
	if got := store1.Get("2"); got == nil || got.Title != "from-teammate" {
		t.Fatalf("store1 did not reload teammate task: %+v", got)
	}
}

func TestSharedTaskInitEmpty(t *testing.T) {
	store := newTestStore(t)
	store.Create("x", "", "", nil, nil, "")
	store.InitEmpty()
	if len(store.ListTasks("", "")) != 0 {
		t.Fatalf("initEmpty did not clear")
	}
	if store.Create("y", "", "", nil, nil, "").ID != "1" {
		t.Fatalf("nextID not reset")
	}
}
