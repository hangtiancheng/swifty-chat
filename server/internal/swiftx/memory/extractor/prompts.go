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

// Package extractor implements the background memory extraction subagent.
//
// The extraction agent runs as a perfect fork of the main conversation — same system prompt, same
// message prefix. The main agent's system prompt always has full save instructions; when the main
// agent writes memories itself, extractor.Execute skips that turn (hasMemoryWritesSince). This
// prompt fires only when the main agent didn't write, so the save-criteria here overlap the system
// prompt's harmlessly.
package extractor

import (
	"fmt"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
)

// Tool names used in the extraction prompt. Match Swiftx's tool registry (capitalized, no
// underscores) — these strings are baked into prompts so the model issues calls that the registry
// recognizes.
const (
	bashToolName      = "Bash"
	fileEditToolName  = "EditFile"
	fileReadToolName  = "ReadFile"
	fileWriteToolName = "WriteFile"
	globToolName      = "Glob"
	grepToolName      = "Grep"
)

// opener is the shared opening section for both extract-prompt variants.
func opener(newMessageCount int, existingMemories string) string {
	manifest := ""
	if existingMemories != "" {
		manifest = fmt.Sprintf(
			"\n\n## Existing memory files\n\n%s\n\nCheck this list before writing — update an existing file rather than creating a duplicate.",
			existingMemories)
	}
	return strings.Join([]string{
		fmt.Sprintf("You are now acting as the memory extraction subagent. Analyze the most recent ~%d messages above and use them to update your persistent memory systems.", newMessageCount),
		"",
		fmt.Sprintf("Available tools: %s, %s, %s, read-only %s (ls/find/cat/stat/wc/head/tail and similar), and %s/%s for paths inside the memory directory only. %s rm is not permitted. All other tools — MCP, Agent, write-capable %s, etc — will be denied.",
			fileReadToolName, grepToolName, globToolName, bashToolName,
			fileEditToolName, fileWriteToolName,
			bashToolName, bashToolName),
		"",
		fmt.Sprintf("You have a limited turn budget. %s requires a prior %s of the same file, so the efficient strategy is: turn 1 — issue all %s calls in parallel for every file you might update; turn 2 — issue all %s/%s calls in parallel. Do not interleave reads and writes across multiple turns.",
			fileEditToolName, fileReadToolName, fileReadToolName, fileWriteToolName, fileEditToolName),
		"",
		fmt.Sprintf("You MUST only use content from the last ~%d messages to update your persistent memories. Do not waste any turns attempting to investigate or verify that content further — no grepping source files, no reading code to confirm a pattern exists, no git commands.%s",
			newMessageCount, manifest),
	}, "\n")
}

// BuildExtractAutoOnlyPrompt builds the extraction prompt for auto-only memory (no team memory).
// Four-type taxonomy with dual-path scoping: user/feedback memories live in userMemDir,
// project/reference memories live in projectMemDir.
//
// userMemDir may be empty (e.g. when $HOME is unset) — in which case the prompt collapses to
// project-only routing and the user/feedback types are effectively unavailable.
func BuildExtractAutoOnlyPrompt(newMessageCount int, existingMemories string, skipIndex bool, userMemDir, projectMemDir string) string {
	var routing strings.Builder
	routing.WriteString("## Memory storage paths\n\n")
	if userMemDir != "" {
		fmt.Fprintf(&routing, "- `user` and `feedback` type → write to `%s` (user-level; follows the human across projects)\n", userMemDir)
	}
	if projectMemDir != "" {
		fmt.Fprintf(&routing, "- `project` and `reference` type → write to `%s` (project-level; lives with this repo)\n", projectMemDir)
	}
	routing.WriteString("\nPick the type first, then write the memory file (and its MEMORY.md pointer) into the matching directory. Never write a `user`/`feedback` memory into the project-level dir or vice versa.")

	var howToSave []string
	if skipIndex {
		howToSave = []string{
			"## How to save memories",
			"",
			"Write each memory to its own file (e.g., `user_role.md`, `feedback_testing.md`) using this frontmatter format:",
			"",
			memory.MemoryFrontmatterExample,
			"",
			"- Organize memory semantically by topic, not chronologically",
			"- Update or remove memories that turn out to be wrong or outdated",
			"- Do not write duplicate memories. First check if there is an existing memory you can update before writing a new one.",
		}
	} else {
		howToSave = []string{
			"## How to save memories",
			"",
			"Saving a memory is a two-step process:",
			"",
			"**Step 1** — write the memory to its own file (e.g., `user_role.md`, `feedback_testing.md`) in the directory that matches its `type` (see Memory storage paths above), using this frontmatter format:",
			"",
			memory.MemoryFrontmatterExample,
			"",
			"**Step 2** — add a pointer to that file in the `MEMORY.md` index in the SAME directory. `MEMORY.md` is an index, not a memory — each entry should be one line, under ~150 characters: `- [Title](file.md) — one-line hook`. It has no frontmatter. Never write memory content directly into `MEMORY.md`.",
			"",
			"- Both `MEMORY.md` files (user-level and project-level) are loaded into context — lines after 200 each will be truncated, so keep each index concise",
			"- Organize memory semantically by topic, not chronologically",
			"- Update or remove memories that turn out to be wrong or outdated",
			"- Do not write duplicate memories. First check if there is an existing memory you can update before writing a new one.",
		}
	}

	parts := []string{
		opener(newMessageCount, existingMemories),
		"",
		"If the user explicitly asks you to remember something, save it immediately as whichever type fits best. If they ask you to forget something, find and remove the relevant entry.",
		"",
		routing.String(),
		"",
		memory.TypesSectionDualPath,
		memory.WhatNotToSaveSection,
		"",
	}
	parts = append(parts, howToSave...)
	return strings.Join(parts, "\n")
}
