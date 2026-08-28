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

package consolidation

import (
	"fmt"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/memory"
)

// BuildConsolidationPrompt builds the full prompt for memory consolidation.
// memoryDir is the project-level memory directory, transcriptDir is the directory containing session JSONL files,
// sessionIDs is the list of session IDs since the last consolidation.
func BuildConsolidationPrompt(memoryDir, userMemoryDir, transcriptDir string, sessionIDs []string) string {
	var sb strings.Builder

	sb.WriteString(`# Dream: Memory Consolidation

You are performing a dream — a reflective pass over your memory files. Synthesize what you've learned recently into durable, well-organized memories so that future sessions can orient quickly.

`)

	sb.WriteString(fmt.Sprintf("Project memory directory: `%s`\n", memoryDir))
	if userMemoryDir != "" {
		sb.WriteString(fmt.Sprintf("User memory directory: `%s`\n", userMemoryDir))
	}
	sb.WriteString(fmt.Sprintf("The memory directory already exists — write to it directly.\n\n"))
	sb.WriteString(fmt.Sprintf("Session transcripts: `%s` (large JSONL files — grep narrowly, don't read whole files)\n\n", transcriptDir))

	sb.WriteString(`---

## Phase 1 — Orient

- ` + "`ls`" + ` the memory directory to see what already exists
- Read ` + "`" + memory.AutoMemEntrypointName + "`" + ` to understand the current index
- Skim existing topic files so you improve them rather than creating duplicates
- If a user memory directory exists, scan it too

## Phase 2 — Gather recent signal

Look for new information worth persisting. Sources in rough priority order:

1. **Existing memories that drifted** — facts that contradict something you see in the codebase now
2. **Transcript search** — if you need specific context, grep the JSONL transcripts for narrow terms:
   ` + "`" + `grep -rn "<narrow term>" ` + transcriptDir + `/ --include="*.jsonl" | tail -50` + "`" + `

Don't exhaustively read transcripts. Look only for things you already suspect matter.

## Phase 3 — Consolidate

For each thing worth remembering, write or update a memory file. Each memory file uses YAML frontmatter with name, description, and metadata.type fields, followed by a Markdown body.

Focus on:
- Merging new signal into existing topic files rather than creating near-duplicates
- Converting relative dates ("yesterday", "last week") to absolute dates so they remain interpretable after time passes
- Deleting contradicted facts — if today's investigation disproves an old memory, fix it at the source

## Phase 4 — Prune and index

Update ` + "`" + memory.AutoMemEntrypointName + "`" + ` so it stays under ` + fmt.Sprintf("%d", memory.MaxEntrypointLines) + ` lines AND under ~25KB. It's an **index**, not a dump — each entry should be one line under ~150 characters: ` + "`- [Title](file.md) — one-line hook`" + `. Never write memory content directly into it.

- Remove pointers to memories that are now stale, wrong, or superseded
- Demote verbose entries: if an index line is over ~200 chars, it's carrying content that belongs in the topic file — shorten the line, move the detail
- Add pointers to newly important memories
- Resolve contradictions — if two files disagree, fix the wrong one

---

**Tool constraints for this run:** Bash is restricted to read-only commands (` + "`ls`" + `, ` + "`find`" + `, ` + "`grep`" + `, ` + "`cat`" + `, ` + "`stat`" + `, ` + "`wc`" + `, ` + "`head`" + `, ` + "`tail`" + `, and similar). Anything that writes, redirects to a file, or modifies state will be denied. Plan your exploration with this in mind.

`)

	if len(sessionIDs) > 0 {
		sb.WriteString(fmt.Sprintf("Sessions since last consolidation (%d):\n", len(sessionIDs)))
		for _, id := range sessionIDs {
			sb.WriteString(fmt.Sprintf("- %s\n", id))
		}
	}

	sb.WriteString("\nReturn a brief summary of what you consolidated, updated, or pruned. If nothing changed (memories are already tight), say so.")

	return sb.String()
}
