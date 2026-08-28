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
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RelevantMemory is one memory file selected for surfacing into the main conversation. MtimeMs is
// threaded through so callers can render freshness without a second stat.
type RelevantMemory struct {
	Path    string
	MtimeMs int64
}

// SelectorFn is the abstraction for the side-query LLM call used by the recall selector. Given a
// system prompt and a user message, the caller is expected to issue a one-shot model call and
// return the raw assistant text. Errors are treated as "selector failed → no recall" by
// FindRelevantMemories. Swiftx's llm.Client is a streaming + system-prompt-bound interface, so
// this callback lets the caller stand up a dedicated side-query client without coupling memory →
// llm at the package level.
type SelectorFn func(ctx context.Context, systemPrompt, userMessage string) (string, error)

// SelectMemoriesSystemPrompt is the system prompt for the selector agent. Client.
const SelectMemoriesSystemPrompt = `You are selecting memories that will be useful to Swiftx as it processes a user's query. You will be given the user's query and a list of available memory files with their filenames and descriptions.

Return a list of filenames for the memories that will clearly be useful to Swiftx as it processes the user's query (up to 5). Only include memories that you are certain will be helpful based on their name and description.
- If you are unsure if a memory will be useful in processing the user's query, then do not include it in your list. Be selective and discerning.
- If there are no memories in the list that would clearly be useful, feel free to return an empty list.
- If a list of recently-used tools is provided, do not select memories that are usage reference or API documentation for those tools (Swiftx is already exercising them). DO still select memories containing warnings, gotchas, or known issues about those tools — active use is exactly when those matter.

Respond with valid JSON only, no markdown, in this exact shape: {"selected_memories": ["filename1.md", "filename2.md"]}`

// FindRelevantMemories scans both userMemDir and projectMemDir, asks the selector to pick up to 5
// relevant filenames for query, and returns the corresponding absolute paths + mtimes. Excludes
// MEMORY.md (already loaded in system prompt). mtime is threaded through so callers can surface
// freshness without a second stat.
//
// alreadySurfaced filters paths shown in prior turns before the selector call, so the 5-slot budget
// is spent on fresh candidates instead of re-picking files the caller will discard.
//
// Either dir may be empty — only the non-empty one is scanned. Filename collisions across the two
// dirs are disambiguated by FilePath in the returned RelevantMemory.
//
// Selector failures are silent — recall is best-effort and must never block the main conversation.
// Returns empty slice + nil error on any selector/parse error.
func FindRelevantMemories(
	ctx context.Context,
	query string,
	userMemDir, projectMemDir string,
	recentTools []string,
	alreadySurfaced map[string]struct{},
	selector SelectorFn,
) ([]RelevantMemory, error) {
	if selector == nil {
		return nil, nil
	}
	var all []MemoryHeader
	if userMemDir != "" {
		userScan, err := ScanMemoryFiles(ctx, userMemDir, "user")
		if err != nil {
			return nil, err
		}
		all = append(all, userScan...)
	}
	if projectMemDir != "" {
		projectScan, err := ScanMemoryFiles(ctx, projectMemDir, "project")
		if err != nil {
			return nil, err
		}
		all = append(all, projectScan...)
	}
	memories := make([]MemoryHeader, 0, len(all))
	for _, m := range all {
		if _, ok := alreadySurfaced[m.FilePath]; ok {
			continue
		}
		memories = append(memories, m)
	}
	if len(memories) == 0 {
		return nil, nil
	}

	selectedFilenames, _ := selectRelevantMemories(ctx, query, memories, recentTools, selector)
	byKey := make(map[string]MemoryHeader, len(memories))
	for _, m := range memories {
		byKey[m.FilePath] = m
		// Also index by Filename for selector outputs that drop the path.
		if _, exists := byKey[m.Filename]; !exists {
			byKey[m.Filename] = m
		}
	}
	selected := make([]RelevantMemory, 0, len(selectedFilenames))
	for _, fn := range selectedFilenames {
		m, ok := byKey[fn]
		if !ok {
			continue
		}
		selected = append(selected, RelevantMemory{Path: m.FilePath, MtimeMs: m.MtimeMs})
	}
	return selected, nil
}

func selectRelevantMemories(
	ctx context.Context,
	query string,
	memories []MemoryHeader,
	recentTools []string,
	selector SelectorFn,
) ([]string, error) {
	validFilenames := make(map[string]struct{}, len(memories))
	for _, m := range memories {
		validFilenames[m.Filename] = struct{}{}
	}

	manifest := FormatMemoryManifest(memories)

	// When Swiftx is actively using a tool (e.g. mcp__X__spawn), surfacing that tool's reference docs
	// is noise — the conversation already contains working usage. The selector otherwise matches on
	// keyword overlap ("spawn" in query + "spawn" in a memory description → false positive).
	toolsSection := ""
	if len(recentTools) > 0 {
		toolsSection = "\n\nRecently used tools: " + strings.Join(recentTools, ", ")
	}

	userMessage := fmt.Sprintf("Query: %s\n\nAvailable memories:\n%s%s", query, manifest, toolsSection)

	raw, err := selector(ctx, SelectMemoriesSystemPrompt, userMessage)
	if err != nil {
		return nil, nil
	}
	clean := extractJSONObject(raw)
	if clean == "" {
		return nil, nil
	}
	var parsed struct {
		SelectedMemories []string `json:"selected_memories"`
	}
	if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
		return nil, nil
	}
	out := make([]string, 0, len(parsed.SelectedMemories))
	for _, f := range parsed.SelectedMemories {
		if _, ok := validFilenames[f]; ok {
			out = append(out, f)
		}
	}
	return out, nil
}

// extractJSONObject returns the first {.} substring found in raw, or the raw text trimmed if it
// already starts with `{`. Tolerates markdown fences or prose around the JSON despite the prompt.
func extractJSONObject(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") {
		return trimmed
	}
	start := strings.Index(trimmed, "{")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(trimmed, "}")
	if end < start {
		return ""
	}
	return trimmed[start : end+1]
}
