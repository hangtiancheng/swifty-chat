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
	"context"
	"fmt"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// TaskCreateTool creates a new task in the todo list.
type TaskCreateTool struct {
	List *TaskList
}

func (t *TaskCreateTool) Name() string { return "TaskCreate" }

func (t *TaskCreateTool) Category() tools.ToolCategory { return tools.CategoryWrite }

func (t *TaskCreateTool) Description() string {
	return "Create a new task to track work. Use this to break complex work into smaller, trackable steps before starting implementation."
}

func (t *TaskCreateTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject":     map[string]any{"type": "string", "description": "A brief title for the task"},
				"description": map[string]any{"type": "string", "description": "What needs to be done"},
				"activeForm": map[string]any{
					"type":        "string",
					"description": "Present continuous form shown in spinner when in_progress (e.g., \"Running tests\")",
				},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Arbitrary metadata to attach to the task",
				},
			},
			"required": []string{"subject", "description"},
		},
	}
}

func (t *TaskCreateTool) Execute(_ context.Context, args map[string]any) tools.ToolResult {
	subject, _ := args["subject"].(string)
	description, _ := args["description"].(string)
	if subject == "" || description == "" {
		return tools.ToolResult{Output: "Error: subject and description are required", IsError: true}
	}

	activeForm, _ := args["activeForm"].(string)
	metadata, _ := args["metadata"].(map[string]any)

	task, err := t.List.Create(subject, description, activeForm, metadata)
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error creating task: %s", err), IsError: true}
	}

	return tools.ToolResult{Output: fmt.Sprintf("Task #%s created successfully: %s", task.ID, task.Subject)}
}

// TaskGetTool retrieves a task by ID.
type TaskGetTool struct {
	List *TaskList
}

func (t *TaskGetTool) Name() string                 { return "TaskGet" }
func (t *TaskGetTool) Category() tools.ToolCategory { return tools.CategoryRead }

func (t *TaskGetTool) Description() string {
	return "Get the details of a specific task by its ID."
}

func (t *TaskGetTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"taskId": map[string]any{"type": "string", "description": "The ID of the task to retrieve"},
			},
			"required": []string{"taskId"},
		},
	}
}

func (t *TaskGetTool) Execute(_ context.Context, args map[string]any) tools.ToolResult {
	taskID, _ := args["taskId"].(string)
	if taskID == "" {
		return tools.ToolResult{Output: "Error: taskId is required", IsError: true}
	}

	task, err := t.List.Get(taskID)
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: %s", err), IsError: true}
	}
	if task == nil {
		return tools.ToolResult{Output: fmt.Sprintf("Task #%s not found", taskID)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Task #%s\n", task.ID)
	fmt.Fprintf(&sb, "Subject: %s\n", task.Subject)
	fmt.Fprintf(&sb, "Status: %s\n", task.Status)
	fmt.Fprintf(&sb, "Description: %s\n", task.Description)
	if len(task.Blocks) > 0 {
		fmt.Fprintf(&sb, "Blocks: %s\n", strings.Join(task.Blocks, ", "))
	}
	if len(task.BlockedBy) > 0 {
		fmt.Fprintf(&sb, "Blocked by: %s\n", strings.Join(task.BlockedBy, ", "))
	}
	if task.Owner != "" {
		fmt.Fprintf(&sb, "Owner: %s\n", task.Owner)
	}
	return tools.ToolResult{Output: sb.String()}
}

// TaskListTool lists all tasks.
type TaskListTool struct {
	List *TaskList
}

func (t *TaskListTool) Name() string                 { return "TaskList" }
func (t *TaskListTool) Category() tools.ToolCategory { return tools.CategoryRead }

func (t *TaskListTool) Description() string {
	return "List all tasks in the current task list. Shows ID, status, subject, and blocking info."
}

func (t *TaskListTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *TaskListTool) Execute(_ context.Context, _ map[string]any) tools.ToolResult {
	tasks, err := t.List.List()
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: %s", err), IsError: true}
	}
	if len(tasks) == 0 {
		return tools.ToolResult{Output: "No tasks found."}
	}

	completedIDs := make(map[string]bool)
	for _, task := range tasks {
		if task.Status == StatusCompleted {
			completedIDs[task.ID] = true
		}
	}

	var sb strings.Builder
	for _, task := range tasks {
		fmt.Fprintf(&sb, "#%s [%s] %s", task.ID, task.Status, task.Subject)
		if task.Owner != "" {
			fmt.Fprintf(&sb, " (owner: %s)", task.Owner)
		}
		var activeBlockers []string
		for _, b := range task.BlockedBy {
			if !completedIDs[b] {
				activeBlockers = append(activeBlockers, b)
			}
		}
		if len(activeBlockers) > 0 {
			fmt.Fprintf(&sb, " [blocked by: %s]", strings.Join(activeBlockers, ", "))
		}
		sb.WriteByte('\n')
	}
	return tools.ToolResult{Output: sb.String()}
}

// TaskUpdateTool updates an existing task.
type TaskUpdateTool struct {
	List *TaskList
}

func (t *TaskUpdateTool) Name() string { return "TaskUpdate" }

func (t *TaskUpdateTool) Category() tools.ToolCategory { return tools.CategoryWrite }

func (t *TaskUpdateTool) Description() string {
	return "Update a task's status, subject, description, or dependencies. Set status to \"in_progress\" when starting work, \"completed\" when done. Set status to \"deleted\" to remove a task."
}

func (t *TaskUpdateTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"taskId":      map[string]any{"type": "string", "description": "The ID of the task to update"},
				"subject":     map[string]any{"type": "string", "description": "New subject for the task"},
				"description": map[string]any{"type": "string", "description": "New description for the task"},
				"activeForm":  map[string]any{"type": "string", "description": "Present continuous form shown in spinner when in_progress"},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"pending", "in_progress", "completed", "deleted"},
					"description": "New status for the task",
				},
				"addBlocks":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Task IDs that this task blocks"},
				"addBlockedBy": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Task IDs that block this task"},
				"owner":        map[string]any{"type": "string", "description": "New owner for the task"},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Metadata keys to merge. Set a key to null to delete it.",
				},
			},
			"required": []string{"taskId"},
		},
	}
}

func (t *TaskUpdateTool) Execute(_ context.Context, args map[string]any) tools.ToolResult {
	taskID, _ := args["taskId"].(string)
	if taskID == "" {
		return tools.ToolResult{Output: "Error: taskId is required", IsError: true}
	}

	task, changed, err := t.List.Update(taskID, args)
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: %s", err), IsError: true}
	}
	if task == nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: task #%s not found", taskID), IsError: true}
	}
	if len(changed) == 0 {
		return tools.ToolResult{Output: fmt.Sprintf("Task #%s: no changes applied", taskID)}
	}

	return tools.ToolResult{
		Output: fmt.Sprintf("Task #%s updated: %s", taskID, strings.Join(changed, ", ")),
	}
}
