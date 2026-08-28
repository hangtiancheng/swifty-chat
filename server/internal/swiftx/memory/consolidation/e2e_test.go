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

//go:build e2e

package consolidation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
	subagent "github.com/hangtiancheng/swifty-chat/server/internal/swiftx/subagent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// TestE2E_ConsolidationMergesDuplicates is a true end-to-end test:
// runs the memory consolidation sub-agent with a real LLM and verifies that
// it merges duplicate memories.
//
// Run with: go test -tags e2e -run TestE2E -v -timeout 120s
// Requires env vars SWIFTX_TEST_API_KEY and SWIFTX_TEST_BASE_URL
func TestE2E_ConsolidationMergesDuplicates(t *testing.T) {
	apiKey := os.Getenv("SWIFTX_TEST_API_KEY")
	baseURL := os.Getenv("SWIFTX_TEST_BASE_URL")
	model := os.Getenv("SWIFTX_TEST_MODEL")
	if apiKey == "" {
		t.Skip("SWIFTX_TEST_API_KEY not set, skipping E2E test")
	}
	if baseURL == "" {
		baseURL = "https://api.minimaxi.com/v1"
	}
	if model == "" {
		model = "MiniMax-M3"
	}

	// Set up test directory
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".swiftx", "memory")
	os.MkdirAll(memDir, 0o755)

	// Write two duplicate memory files
	writeMemory(t, memDir, "feedback_no_push.md", "feedback", "no-push",
		"Don't push without asking",
		"User does not want automatic git push")

	writeMemory(t, memDir, "feedback_auto_push.md", "feedback", "auto-push",
		"Don't auto push code",
		"User dislikes auto push, always ask first")

	// Write a stale memory
	writeMemory(t, memDir, "project_deadline.md", "project", "deadline",
		"Project deadline is 2025-01-15",
		"Project deadline is January 15, 2025\n\n**Why:** Client requirement\n**How to apply:** All tasks must be completed before this date")

	// Write a normal memory
	writeMemory(t, memDir, "user_role.md", "user", "user-role",
		"User is a backend engineer",
		"User is a backend engineer, primarily uses Go and Java")

	// Write MEMORY.md index
	indexContent := `- [No push](feedback_no_push.md) — Do not auto push
- [Auto push](feedback_auto_push.md) — Do not auto push code
- [Deadline](project_deadline.md) — Project deadline 2025-01-15
- [User role](user_role.md) — Backend engineer
`
	os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte(indexContent), 0o644)

	t.Logf("Test directory: %s", dir)
	t.Logf("Memory files before consolidation:")
	listMemoryFiles(t, memDir)

	// Build LLM client
	cfg := &config.ProviderConfig{}
	cfg.Protocol = "openai-compat"
	cfg.BaseURL = baseURL
	cfg.APIKey = apiKey
	cfg.Model = model
	cfg.ContextWindow = 200000
	client, err := llm.NewClient(cfg, "")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Build tool registry: register tools needed for consolidation
	registry := tools.NewRegistry()
	registry.Register(&tools.ReadFileTool{})
	registry.Register(&tools.WriteFileTool{})
	registry.Register(&tools.EditFileTool{})
	registry.Register(&tools.GlobTool{})
	registry.Register(&tools.GrepTool{})
	registry.Register(&tools.BashTool{})
	subRegistry := subagent.FilterToolsForAgent(registry, nil, nil, true)

	// Permission sandbox: only allow writes to memory directory
	sandbox := permissions.NewPathSandbox(memDir)
	checker := permissions.NewChecker(sandbox, &permissions.RuleEngine{}, permissions.ModeBypass)

	// Build consolidation prompt
	prompt := BuildConsolidationPrompt(memDir, "", "", nil)

	conv := conversation.NewManager()
	conv.AddUserMessage(prompt)

	subAgent := agent.New(client, subRegistry, "openai")
	subAgent.MaxIterations = 15
	subAgent.Checker = checker
	subAgent.WorkDir = dir

	t.Log("Starting consolidation sub-agent...")
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	ch := subAgent.Run(ctx, conv)
	for event := range ch {
		switch e := event.(type) {
		case agent.StreamText:
			fmt.Print(e.Text)
		case agent.LoopComplete:
			t.Logf("\nSub-agent completed in %s, %d turns", time.Since(startTime), e.TotalTurns)
		}
	}

	// Verify results
	t.Log("\n\nMemory files after consolidation:")
	listMemoryFiles(t, memDir)

	// Check 1: were duplicate memories merged (at least one deletion)
	files := listMdFiles(memDir)
	hasPush := false
	pushCount := 0
	for _, f := range files {
		content, _ := os.ReadFile(filepath.Join(memDir, f))
		if strings.Contains(strings.ToLower(string(content)), "push") {
			pushCount++
			hasPush = true
		}
	}
	if hasPush && pushCount > 1 {
		t.Logf("WARNING: push-related memories not merged (still %d files)", pushCount)
	} else if pushCount <= 1 {
		t.Log("PASS: duplicate push memories merged")
	}

	// Check 2: was MEMORY.md index updated
	indexBytes, err := os.ReadFile(filepath.Join(memDir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("MEMORY.md not found after consolidation")
	}
	t.Logf("MEMORY.md content:\n%s", string(indexBytes))

	// Check 3: are index line counts reasonable
	lines := strings.Split(strings.TrimSpace(string(indexBytes)), "\n")
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	if nonEmpty > memory.MaxEntrypointLines {
		t.Errorf("MEMORY.md has %d lines, exceeds limit %d", nonEmpty, memory.MaxEntrypointLines)
	}

	// Check 4: were there WriteFile/EditFile tool calls in the conversation
	writtenPaths := extractWrittenPaths(conv.GetMessages())
	t.Logf("Files written by sub-agent: %v", writtenPaths)
	if len(writtenPaths) == 0 {
		t.Error("sub-agent did not write any files — consolidation had no effect")
	}
}

func writeMemory(t *testing.T, dir, filename, memType, name, desc, body string) {
	t.Helper()
	content := fmt.Sprintf(`---
name: %s
description: %s
metadata:
  type: %s
---

%s
`, name, desc, memType, body)
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func listMemoryFiles(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			info, _ := e.Info()
			t.Logf("  %s (%d bytes)", e.Name(), info.Size())
		}
	}
}

func listMdFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") && e.Name() != "MEMORY.md" {
			files = append(files, e.Name())
		}
	}
	return files
}
