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
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/worktree"
)

// EnterWorktreeTool creates an isolated git worktree and switches the session into it.
type EnterWorktreeTool struct {
	SessionID string // injected by TUI at startup
	RepoRoot  string // injected by TUI at startup
}

func (t *EnterWorktreeTool) Name() string { return "EnterWorktree" }

func (t *EnterWorktreeTool) Category() ToolCategory { return CategoryCommand }

func (t *EnterWorktreeTool) Description() string {
	return "Creates an isolated worktree (via git) and switches the session into it"
}

func (t *EnterWorktreeTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": `Optional name for the worktree. Each "/"-separated segment may contain only letters, digits, dots, underscores, and dashes; max 64 chars total. A random name is generated if not provided.`,
				},
			},
		},
	}
}

func (t *EnterWorktreeTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	// Guard: reject if already in a worktree session.
	if worktree.GetCurrentWorktreeSession() != nil {
		return ToolResult{
			Output:  "Already in a worktree session",
			IsError: true,
		}
	}

	slug, _ := args["name"].(string)
	if slug == "" {
		slug = generateWorktreeSlug()
	}

	repoRoot := t.RepoRoot
	if repoRoot == "" {
		return ToolResult{
			Output:  "Error: not in a git repository",
			IsError: true,
		}
	}

	session, err := worktree.CreateWorktreeForSession(ctx, t.SessionID, slug, repoRoot)
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Error creating worktree: %s", err),
			IsError: true,
		}
	}

	branchInfo := ""
	if session.WorktreeBranch != "" {
		branchInfo = " on branch " + session.WorktreeBranch
	}

	return ToolResult{
		Output: fmt.Sprintf(
			"Created worktree at %s%s. The session is now working in the worktree. Use ExitWorktree to leave mid-session, or exit the session to be prompted.",
			session.WorktreePath, branchInfo,
		),
	}
}

func generateWorktreeSlug() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "wt-" + hex.EncodeToString(b)
}
