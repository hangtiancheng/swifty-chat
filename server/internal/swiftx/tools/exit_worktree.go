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

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/worktree"
)

// ExitWorktreeTool exits a worktree session created by EnterWorktree and restores the original
// working directory.
type ExitWorktreeTool struct {
	RepoRoot string // injected by TUI at startup
}

func (t *ExitWorktreeTool) Name() string { return "ExitWorktree" }

func (t *ExitWorktreeTool) Category() ToolCategory { return CategoryCommand }

func (t *ExitWorktreeTool) Description() string {
	return "Exits a worktree session created by EnterWorktree and restores the original working directory"
}

func (t *ExitWorktreeTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"keep", "remove"},
					"description": `"keep" leaves the worktree and branch on disk; "remove" deletes both.`,
				},
				"discard_changes": map[string]any{
					"type":        "boolean",
					"description": `Required true when action is "remove" and the worktree has uncommitted files or unmerged commits. The tool will refuse and list them otherwise.`,
				},
			},
			"required": []string{"action"},
		},
	}
}

func (t *ExitWorktreeTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	// Scope guard: only operates on worktrees created by EnterWorktree in THIS session.
	session := worktree.GetCurrentWorktreeSession()
	if session == nil {
		return ToolResult{
			Output:  "No-op: there is no active EnterWorktree session to exit. This tool only operates on worktrees created by EnterWorktree in the current session — it will not touch worktrees created manually or in a previous session. No filesystem changes were made.",
			IsError: true,
		}
	}

	action, _ := args["action"].(string)
	discardChanges, _ := args["discard_changes"].(bool)

	repoRoot := t.RepoRoot
	if repoRoot == "" {
		repoRoot = session.OriginalCwd
	}

	// Validate: if removing without discard_changes, check for changes.
	if action == "remove" && !discardChanges {
		summary := worktree.CountWorktreeChanges(ctx, session.WorktreePath, session.OriginalHeadCommit)
		if summary == nil {
			return ToolResult{
				Output: fmt.Sprintf(
					"Could not verify worktree state at %s. Refusing to remove without explicit confirmation. Re-invoke with discard_changes: true to proceed — or use action: \"keep\" to preserve the worktree.",
					session.WorktreePath,
				),
				IsError: true,
			}
		}
		if summary.ChangedFiles > 0 || summary.Commits > 0 {
			var parts []string
			if summary.ChangedFiles > 0 {
				word := "files"
				if summary.ChangedFiles == 1 {
					word = "file"
				}
				parts = append(parts, fmt.Sprintf("%d uncommitted %s", summary.ChangedFiles, word))
			}
			if summary.Commits > 0 {
				word := "commits"
				if summary.Commits == 1 {
					word = "commit"
				}
				branchName := session.WorktreeBranch
				if branchName == "" {
					branchName = "the worktree branch"
				}
				parts = append(parts, fmt.Sprintf("%d %s on %s", summary.Commits, word, branchName))
			}
			return ToolResult{
				Output: fmt.Sprintf(
					"Worktree has %s. Removing will discard this work permanently. Confirm with the user, then re-invoke with discard_changes: true — or use action: \"keep\" to preserve the worktree.",
					strings.Join(parts, " and "),
				),
				IsError: true,
			}
		}
	}

	// Capture session info before cleanup.
	originalCwd := session.OriginalCwd
	worktreePath := session.WorktreePath
	worktreeBranch := session.WorktreeBranch

	if action == "keep" {
		if err := worktree.KeepWorktree(repoRoot); err != nil {
			return ToolResult{
				Output:  fmt.Sprintf("Error keeping worktree: %s", err),
				IsError: true,
			}
		}

		branchInfo := ""
		if worktreeBranch != "" {
			branchInfo = " on branch " + worktreeBranch
		}
		return ToolResult{
			Output: fmt.Sprintf(
				"Exited worktree. Your work is preserved at %s%s. Session is now back in %s.",
				worktreePath, branchInfo, originalCwd,
			),
		}
	}

	// action == "remove".
	if err := worktree.CleanupWorktree(ctx, repoRoot); err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Error removing worktree: %s", err),
			IsError: true,
		}
	}

	return ToolResult{
		Output: fmt.Sprintf(
			"Exited and removed worktree at %s. Session is now back in %s.",
			worktreePath, originalCwd,
		),
	}
}
