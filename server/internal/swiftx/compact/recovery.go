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

package compact

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Recovery limits for the attachment block that gets appended to the
// summary message. Compact wipes the working conversation; without these
// snapshots the model would forget which files it just read and which
// skill SOPs it was operating under.
const (
	RecoveryFileLimit      = 5
	RecoveryTokensPerFile  = 5_000
	RecoverySkillsBudget   = 25_000
	RecoveryTokensPerSkill = 5_000
	recoveryCharsPerToken  = 3.5
)

// FileReadRecord snapshots the bytes a ReadFile call returned to the
// model. Re-injected post-compact so the model still has the content it
// was reasoning about when the threshold tripped.
type FileReadRecord struct {
	Path      string
	Content   string
	Timestamp time.Time
}

// SkillInvocationRecord captures the SOP body that was attached when a
// skill was invoked. After compaction the same definition gets stitched
// back in so behaviour stays consistent across the boundary.
type SkillInvocationRecord struct {
	Name      string
	Body      string
	Timestamp time.Time
}

// RecoveryState tracks the per-agent data that needs to survive
// compaction. The struct is safe for concurrent recording — tool
// callbacks can fire from parallel goroutines in the streaming
// executor.
type RecoveryState struct {
	mu     sync.Mutex
	files  map[string]FileReadRecord
	skills map[string]SkillInvocationRecord
}

// NewRecoveryState returns an empty state ready for recording.
func NewRecoveryState() *RecoveryState {
	return &RecoveryState{
		files:  map[string]FileReadRecord{},
		skills: map[string]SkillInvocationRecord{},
	}
}

// RecordFileRead overwrites any prior record for the same path so the
// most recent snapshot wins. Safe to call on a nil receiver.
func (s *RecoveryState) RecordFileRead(path, content string) {
	if s == nil || path == "" {
		return
	}
	s.mu.Lock()
	s.files[path] = FileReadRecord{Path: path, Content: content, Timestamp: time.Now()}
	s.mu.Unlock()
}

// RecordSkillInvocation overwrites any prior record for the same skill
// name. Safe to call on a nil receiver.
func (s *RecoveryState) RecordSkillInvocation(name, body string) {
	if s == nil || name == "" {
		return
	}
	s.mu.Lock()
	s.skills[name] = SkillInvocationRecord{Name: name, Body: body, Timestamp: time.Now()}
	s.mu.Unlock()
}

// snapshotFiles returns at most `limit` records, newest first.
func (s *RecoveryState) snapshotFiles(limit int) []FileReadRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]FileReadRecord, 0, len(s.files))
	for _, r := range s.files {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// snapshotSkills returns every recorded skill, newest first.
func (s *RecoveryState) snapshotSkills() []SkillInvocationRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SkillInvocationRecord, 0, len(s.skills))
	for _, r := range s.skills {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	return out
}

// BuildRecoveryAttachment renders the post-compact recovery sections
// (recently read files, skill definitions, tool listing, plus a closing
// note about not guessing from the summary) into a single block of text.
// Returns "" when there is nothing worth emitting so the caller can keep
// the summary message clean.
func BuildRecoveryAttachment(state *RecoveryState, toolSchemas []map[string]any) string {
	var sb strings.Builder

	if files := state.snapshotFiles(RecoveryFileLimit); len(files) > 0 {
		sb.WriteString("## Recently read files\n\n")
		sb.WriteString("These snapshots are what the file-reading tool last returned. Re-open with the tool if you need the current bytes.\n\n")
		for _, f := range files {
			content := truncateByTokens(f.Content, RecoveryTokensPerFile)
			ts := f.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
			fmt.Fprintf(&sb, "### %s  (read %s)\n\n", f.Path, ts)
			sb.WriteString("```\n")
			sb.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				sb.WriteByte('\n')
			}
			sb.WriteString("```\n\n")
		}
	}

	if skills := state.snapshotSkills(); len(skills) > 0 {
		var section strings.Builder
		section.WriteString("## Active skills\n\n")
		section.WriteString("These skills were invoked earlier in the session. Continue to follow each SOP when its triggering condition applies.\n\n")
		used := 0
		emitted := false
		for _, sk := range skills {
			body := truncateByTokens(sk.Body, RecoveryTokensPerSkill)
			tokens := approxTokens(body) + approxTokens(sk.Name) + 8
			if used+tokens > RecoverySkillsBudget {
				break
			}
			used += tokens
			fmt.Fprintf(&section, "### %s\n\n%s\n\n", sk.Name, body)
			emitted = true
		}
		if emitted {
			sb.WriteString(section.String())
		}
	}

	if len(toolSchemas) > 0 {
		sb.WriteString("## Available tools\n\nYou still have access to the following tools — call them directly when the task needs one:\n\n")
		for _, t := range toolSchemas {
			name, _ := t["name"].(string)
			if name == "" {
				continue
			}
			desc, _ := t["description"].(string)
			desc = firstLine(desc)
			if desc != "" {
				fmt.Fprintf(&sb, "- %s — %s\n", name, desc)
			} else {
				fmt.Fprintf(&sb, "- %s\n", name)
			}
		}
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		return ""
	}

	sb.WriteString("## Note\n\nEverything above the divider is reconstructed context. For exact code, error strings, or user-typed text, re-read the source rather than guess from the summary.\n")
	return sb.String()
}

// approxTokens uses the same chars-per-token heuristic as EstimateTokens
// so budgeting stays consistent across the package.
func approxTokens(s string) int {
	if s == "" {
		return 0
	}
	return int(float64(len(s)) / recoveryCharsPerToken)
}

// truncateByTokens cuts s at the byte offset that puts it just under the
// token budget and appends a marker so the model can see content was
// clipped.
func truncateByTokens(s string, tokenBudget int) string {
	if tokenBudget <= 0 || s == "" {
		return s
	}
	if approxTokens(s) <= tokenBudget {
		return s
	}
	maxChars := int(float64(tokenBudget) * recoveryCharsPerToken)
	if maxChars <= 0 || maxChars >= len(s) {
		return s
	}
	return s[:maxChars] + "\n… (content truncated)"
}

// firstLine returns the first non-empty line of s, trimmed. Used to keep
// the tool listing compact when descriptions are multi-paragraph.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
