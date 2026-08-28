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
package memory

import (
	"fmt"
	"os"
	"strings"
)

// Builds the typed-memory behavioural prompt and the MEMORY.md entrypoint content for inclusion in
// the system prompt.

const (
	// MaxEntrypointLines caps how much of MEMORY.md is loaded into context. ~125 chars/line × 200
	// lines puts us at the observed p97 today.
	MaxEntrypointLines = 200
	// MaxEntrypointBytes catches long-line indexes that slip past the line cap (p100 observed: 197KB
	// under 200 lines).
	MaxEntrypointBytes = 25_000

	autoMemDisplayName = "auto memory"

	// DirExistsGuidance is shipped because the model was burning turns on `ls`/`mkdir -p` before
	// writing. The harness guarantees the directory exists via EnsureMemoryDirExists so the prompt
	// text can promise it.
	DirExistsGuidance = "This directory already exists — write to it directly with the Write tool (do not run mkdir or check for its existence)."
)

// EntrypointTruncation is the result of capping MEMORY.md content.
type EntrypointTruncation struct {
	Content          string
	LineCount        int
	ByteCount        int
	WasLineTruncated bool
	WasByteTruncated bool
}

// TruncateEntrypointContent caps MEMORY.md content to the line AND byte caps, appending a warning
// that names which cap fired. Line-truncates first (natural boundary), then byte-truncates at the
// last newline before the cap so we don't cut mid-line.
func TruncateEntrypointContent(raw string) EntrypointTruncation {
	trimmed := strings.TrimSpace(raw)
	contentLines := strings.Split(trimmed, "\n")
	lineCount := len(contentLines)
	byteCount := len(trimmed)

	wasLineTruncated := lineCount > MaxEntrypointLines
	wasByteTruncated := byteCount > MaxEntrypointBytes

	if !wasLineTruncated && !wasByteTruncated {
		return EntrypointTruncation{
			Content:          trimmed,
			LineCount:        lineCount,
			ByteCount:        byteCount,
			WasLineTruncated: wasLineTruncated,
			WasByteTruncated: wasByteTruncated,
		}
	}

	truncated := trimmed
	if wasLineTruncated {
		truncated = strings.Join(contentLines[:MaxEntrypointLines], "\n")
	}

	if len(truncated) > MaxEntrypointBytes {
		cutAt := strings.LastIndex(truncated[:MaxEntrypointBytes], "\n")
		if cutAt > 0 {
			truncated = truncated[:cutAt]
		} else {
			// No newline in the entire segment — hard-cut required. Back up to a
			// valid UTF-8 character boundary: continuation bytes always have the
			// high two bits set to 10, so skip backwards past them to avoid
			// splitting a multi-byte character.
			end := MaxEntrypointBytes
			for end > 0 && truncated[end]&0xC0 == 0x80 {
				end--
			}
			truncated = truncated[:end]
		}
	}

	var reason string
	switch {
	case wasByteTruncated && !wasLineTruncated:
		reason = fmt.Sprintf("%s (limit: %s) — index entries are too long",
			formatFileSize(byteCount), formatFileSize(MaxEntrypointBytes))
	case wasLineTruncated && !wasByteTruncated:
		reason = fmt.Sprintf("%d lines (limit: %d)", lineCount, MaxEntrypointLines)
	default:
		reason = fmt.Sprintf("%d lines and %s", lineCount, formatFileSize(byteCount))
	}

	return EntrypointTruncation{
		Content: truncated + fmt.Sprintf(
			"\n\n> WARNING: %s is %s. Only part of it was loaded. Keep index entries to one line under ~200 chars; move detail into topic files.",
			AutoMemEntrypointName, reason),
		LineCount:        lineCount,
		ByteCount:        byteCount,
		WasLineTruncated: wasLineTruncated,
		WasByteTruncated: wasByteTruncated,
	}
}

func formatFileSize(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/1024/1024)
	}
}

// EnsureMemoryDirExists creates the memory directory (and parents) so the model can Write into it
// without first running ls/mkdir. Idempotent. Failures are non-fatal — the prompt builder continues
// either way; the model's Write will surface a real permission error if there is one.
func EnsureMemoryDirExists(memoryDir string) error {
	if memoryDir == "" {
		return nil
	}
	return os.MkdirAll(memoryDir, 0o755)
}

// BuildMemoryLines builds the typed-memory behavioural instructions (without MEMORY.md content).
// Constrains memories to a closed four-type taxonomy (user / feedback / project / reference) —
// content that is derivable from the current project state (code patterns, architecture, git
// history) is explicitly excluded.
//
// Dual-path variant: user/feedback memories are written to userMemDir (follows the human across
// projects), project/reference memories are written to projectMemDir (lives with the repo).
// Either dir may be empty — in which case its half of the taxonomy is implicitly disabled.
func BuildMemoryLines(displayName, userMemDir, projectMemDir string) string {
	howToSave := `## How to save memories

Saving a memory is a two-step process:

**Step 1** — write the memory to its own file (e.g., ` + "`user_role.md`" + `, ` + "`feedback_testing.md`" + `) using this frontmatter format:

` + MemoryFrontmatterExample + `

**Step 2** — add a pointer to that file in the ` + "`" + AutoMemEntrypointName + "`" + ` index in the SAME directory as the memory file. ` + "`" + AutoMemEntrypointName + "`" + ` is an index, not a memory — each entry should be one line, under ~150 characters: ` + "`- [Title](file.md) — one-line hook`" + `. It has no frontmatter. Never write memory content directly into ` + "`" + AutoMemEntrypointName + "`" + `.

- Both ` + "`" + AutoMemEntrypointName + "`" + ` files (user-level and project-level) are always loaded into your conversation context` + fmt.Sprintf(" — lines after %d each will be truncated, so keep each index concise", MaxEntrypointLines) + `
- Keep the name, description, and type fields in memory files up-to-date with the content
- Organize memory semantically by topic, not chronologically
- Update or remove memories that turn out to be wrong or outdated
- Do not write duplicate memories. First check if there is an existing memory you can update before writing a new one.`

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", displayName)
	b.WriteString("You have a persistent, file-based memory system organized into two locations by content type:\n\n")
	if userMemDir != "" {
		fmt.Fprintf(&b, "- **User-level** (`%s`) — memories with `type: user` or `type: feedback`. These follow you across all projects, because they describe the human or how the human likes to work. %s\n", userMemDir, DirExistsGuidance)
	}
	if projectMemDir != "" {
		fmt.Fprintf(&b, "- **Project-level** (`%s`) — memories with `type: project` or `type: reference`. These belong to the current repo, can be committed for team sharing or git-ignored for personal use. %s\n", projectMemDir, DirExistsGuidance)
	}
	b.WriteString("\nThe `type` field in each memory file's frontmatter determines which directory it belongs to — pick the type first, then write to the matching directory.\n\n")
	b.WriteString("You should build up this memory system over time so that future conversations can have a complete picture of who the user is, how they'd like to collaborate with you, what behaviors to avoid or repeat, and the context behind the work the user gives you.\n\n")
	b.WriteString("If the user explicitly asks you to remember something, save it immediately as whichever type fits best (and in whichever directory that type belongs to). If they ask you to forget something, find and remove the relevant entry.\n\n")
	b.WriteString(TypesSectionDualPath)
	b.WriteByte('\n')
	b.WriteString(WhatNotToSaveSection)
	b.WriteString("\n\n")
	b.WriteString(howToSave)
	b.WriteString("\n\n")
	b.WriteString(WhenToAccessSection)
	b.WriteString("\n\n")
	b.WriteString(TrustingRecallSection)
	b.WriteString("\n\n")
	b.WriteString("## Memory and other forms of persistence\n")
	b.WriteString("Memory is one of several persistence mechanisms available to you as you assist the user in a given conversation. The distinction is often that memory can be recalled in future conversations and should not be used for persisting information that is only useful within the scope of the current conversation.\n")
	b.WriteString("- When to use or update a plan instead of memory: If you are about to start a non-trivial implementation task and would like to reach alignment with the user on your approach you should use a Plan rather than saving this information to memory. Similarly, if you already have a plan within the conversation and you have changed your approach persist that change by updating the plan rather than saving a memory.\n")
	b.WriteString("- When to use or update tasks instead of memory: When you need to break your work in current conversation into discrete steps or keep track of your progress use tasks instead of saving to memory. Tasks are great for persisting information about the work that needs to be done in the current conversation, but memory should be reserved for information that will be useful in future conversations.")

	return b.String()
}

// BuildMemoryPrompt builds the typed-memory prompt with the user-level and project-level MEMORY.md
// indexes included. Used by the system-prompt assembly so the agent has both the behavioural rules
// and the current indexes in one section.
func BuildMemoryPrompt(displayName, userMemDir, projectMemDir string) string {
	lines := BuildMemoryLines(displayName, userMemDir, projectMemDir)

	var b strings.Builder
	b.WriteString(lines)
	b.WriteString("\n\n")
	if userMemDir != "" {
		writeEntrypointSection(&b, "User-level", userMemDir+AutoMemEntrypointName)
	}
	if projectMemDir != "" {
		if userMemDir != "" {
			b.WriteString("\n\n")
		}
		writeEntrypointSection(&b, "Project-level", projectMemDir+AutoMemEntrypointName)
	}
	return b.String()
}

func writeEntrypointSection(b *strings.Builder, scopeLabel, entrypointPath string) {
	fmt.Fprintf(b, "## %s %s (`%s`)\n\n", scopeLabel, AutoMemEntrypointName, entrypointPath)
	data, err := os.ReadFile(entrypointPath)
	if err == nil && strings.TrimSpace(string(data)) != "" {
		t := TruncateEntrypointContent(string(data))
		b.WriteString(t.Content)
	} else {
		fmt.Fprintf(b, "This %s is currently empty. When you save new %s-level memories, add their pointers here.", AutoMemEntrypointName, strings.ToLower(scopeLabel))
	}
}

// LoadAutoMemoryPrompt is the convenience entrypoint used by the system prompt builder. Ensures the
// user-level and project-level memory directories exist, then returns the assembled `# auto memory`
// section ready for inclusion as a system-prompt section.
func LoadAutoMemoryPrompt(projectRoot string) string {
	userDir := GetUserAutoMemPath()
	projectDir := GetAutoMemPath(projectRoot)
	if userDir == "" && projectDir == "" {
		return ""
	}
	if userDir != "" {
		_ = EnsureMemoryDirExists(userDir)
	}
	if projectDir != "" {
		_ = EnsureMemoryDirExists(projectDir)
	}
	return BuildMemoryPrompt(autoMemDisplayName, userDir, projectDir)
}
