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

package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

// TypeCompactBoundary marks a session record as a compaction boundary rather
// than a plain conversation message. A boundary record's Content holds a JSON
// blob (see CompactBoundary) carrying the summary text plus the recent tail
// (keep) that was preserved verbatim at compaction time. Plain messages leave
// Type empty (omitempty), so old sessions and normal turns are unaffected.
const TypeCompactBoundary = "compact_boundary"

// ToolUseRecord is the on-disk form of a tool invocation. It stores a
// protocol-agnostic internal representation rather than any single vendor's
// wire format, so a session can be restored even after switching providers.
type ToolUseRecord struct {
	ToolUseID string         `json:"tool_use_id"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolResultRecord is the on-disk form of a tool result, paired with a
// ToolUseRecord via ToolUseID.
type ToolResultRecord struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type Message struct {
	Role string `json:"role"`
	// Type distinguishes record kinds. Empty (the default, omitted from JSON)
	// means a plain conversation message; TypeCompactBoundary means Content is a
	// CompactBoundary JSON blob written by SaveCompactBoundary.
	Type    string `json:"type,omitempty"`
	Content string `json:"content"`
	Ts      int64  `json:"ts"`
	// ToolUses / ToolResults hold the tool blocks carried by this message. When both
	// are empty the fields are omitted from JSON entirely, so session files lacking
	// these fields still load fine — just without a tool chain.
	ToolUses    []ToolUseRecord    `json:"tool_uses,omitempty"`
	ToolResults []ToolResultRecord `json:"tool_results,omitempty"`
}

// FromConversation converts an in-memory conversation message into its on-disk form.
// Thinking blocks are not persisted: their signature is only needed when echoed back
// within the same tool loop, and is useless for cross-session restoration.
func FromConversation(msg conversation.Message) Message {
	rec := Message{
		Role:    msg.Role,
		Content: msg.Content,
		Ts:      time.Now().Unix(),
	}
	for _, tu := range msg.ToolUses {
		rec.ToolUses = append(rec.ToolUses, ToolUseRecord{
			ToolUseID: tu.ToolUseID,
			ToolName:  tu.ToolName,
			Arguments: tu.Arguments,
		})
	}
	for _, tr := range msg.ToolResults {
		rec.ToolResults = append(rec.ToolResults, ToolResultRecord{
			ToolUseID: tr.ToolUseID,
			Content:   tr.Content,
			IsError:   tr.IsError,
		})
	}
	return rec
}

// ToConversation restores an on-disk record back into an in-memory conversation
// message, used by resume to rebuild history.
func (m Message) ToConversation() conversation.Message {
	msg := conversation.Message{
		Role:    m.Role,
		Content: m.Content,
	}
	for _, tu := range m.ToolUses {
		msg.ToolUses = append(msg.ToolUses, conversation.ToolUseBlock{
			ToolUseID: tu.ToolUseID,
			ToolName:  tu.ToolName,
			Arguments: tu.Arguments,
		})
	}
	for _, tr := range m.ToolResults {
		msg.ToolResults = append(msg.ToolResults, conversation.ToolResultBlock{
			ToolUseID: tr.ToolUseID,
			Content:   tr.Content,
			IsError:   tr.IsError,
		})
	}
	return msg
}

// KeepMessage is a recent message preserved verbatim when compaction occurs. Like
// Message, it carries tool blocks so that restoring a compacted session keeps this
// tail's tool call chain intact.
type KeepMessage struct {
	Role        string             `json:"role"`
	Content     string             `json:"content"`
	ToolUses    []ToolUseRecord    `json:"tool_uses,omitempty"`
	ToolResults []ToolResultRecord `json:"tool_results,omitempty"`
}

// FromConversationKeep converts a kept tail message into its on-disk form.
func FromConversationKeep(msg conversation.Message) KeepMessage {
	rec := FromConversation(msg)
	return KeepMessage{
		Role:        rec.Role,
		Content:     rec.Content,
		ToolUses:    rec.ToolUses,
		ToolResults: rec.ToolResults,
	}
}

// CompactBoundary is the structured payload stored (as JSON) in the Content of a
// TypeCompactBoundary record. Summary is the LLM-produced summary of the
// older prefix; Keep is the recent tail that was kept verbatim. On resume the
// compacted state is rebuilt as: [user message = Summary] + Keep + any plain
// messages appended after the boundary.
type CompactBoundary struct {
	Summary string        `json:"summary"`
	Keep    []KeepMessage `json:"keep"`
}

// SaveCompactBoundary appends a compaction boundary record to the session log.
// The boundary is append-only: the original prefix messages stay in the file but
// are not replayed on resume (see FindLastCompactBoundary). The summary + keep
// are inlined into the record's Content as a CompactBoundary JSON blob.
func SaveCompactBoundary(workDir, sessionID, summary string, keep []KeepMessage) {
	blob, err := json.Marshal(CompactBoundary{Summary: summary, Keep: keep})
	if err != nil {
		return
	}
	SaveMessage(workDir, sessionID, Message{
		Role:    "system",
		Type:    TypeCompactBoundary,
		Content: string(blob),
		Ts:      time.Now().Unix(),
	})
}

// FindLastCompactBoundary scans the loaded records for the last compaction
// boundary. It returns the parsed boundary, the slice of plain messages appended
// after that boundary, and ok=true when a boundary was found. When no boundary
// exists (ok=false) the caller should replay all records verbatim
// (backward-compatible: old sessions have no boundary records).
func FindLastCompactBoundary(msgs []Message) (boundary CompactBoundary, after []Message, ok bool) {
	last := -1
	for i, m := range msgs {
		if m.Type == TypeCompactBoundary {
			last = i
		}
	}
	if last < 0 {
		return CompactBoundary{}, nil, false
	}
	if err := json.Unmarshal([]byte(msgs[last].Content), &boundary); err != nil {
		// Corrupt boundary blob — fall back to full replay rather than losing
		// the conversation.
		return CompactBoundary{}, nil, false
	}
	for _, m := range msgs[last+1:] {
		if m.Type == TypeCompactBoundary {
			continue // defensive; FindLast already targeted the final one
		}
		after = append(after, m)
	}
	return boundary, after, true
}

type SessionInfo struct {
	ID           string
	FirstMessage string
	MessageCount int
	FileSize     int64
	GitBranch    string
	ModTime      time.Time
}

func NewID() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand rarely fails; fall back to the low 16 bits of the nanosecond
		// clock, which still avoids collisions within the same second and process.
		return fmt.Sprintf("%s-%04x", time.Now().Format("20060102-150405"), time.Now().UnixNano()&0xFFFF)
	}
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

func sessionsDir(workDir string) string {
	return filepath.Join(workDir, ".swiftx", "sessions")
}

func SessionFilePath(workDir, id string) string {
	return filepath.Join(sessionsDir(workDir), id+".jsonl")
}

func SaveMessage(workDir, sessionID string, msg Message) {
	dir := sessionsDir(workDir)
	os.MkdirAll(dir, 0o755)

	f, err := os.OpenFile(SessionFilePath(workDir, sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	data, _ := json.Marshal(msg)
	f.Write(data)
	f.Write([]byte("\n"))
}

func LoadSession(workDir, sessionID string) []Message {
	path := SessionFilePath(workDir, sessionID)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var msgs []Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		var msg Message
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			continue
		}
		// A message carrying only tool results has no text content of its own; it must
		// not be filtered by whether Content is empty, or the entire tool round-trip
		// would be dropped when the session is restored.
		if msg.Content == "" && len(msg.ToolUses) == 0 && len(msg.ToolResults) == 0 {
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// maxSessionAgeDays is the maximum retention period for sessions; sessions older
// than this are cleaned up automatically.
const maxSessionAgeDays = 30

func ListSessions(workDir string) []SessionInfo {
	dir := sessionsDir(workDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	branch := currentGitBranch(workDir)
	cutoff := time.Now().AddDate(0, 0, -maxSessionAgeDays)

	var sessions []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Automatically clean up expired sessions older than 30 days.
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
			continue
		}

		msgs := LoadSession(workDir, id)
		first := ""
		for _, msg := range msgs {
			if msg.Role == "user" {
				first = msg.Content
				break
			}
		}

		sessions = append(sessions, SessionInfo{
			ID:           id,
			FirstMessage: first,
			MessageCount: len(msgs),
			FileSize:     info.Size(),
			GitBranch:    branch,
			ModTime:      info.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	return sessions
}

func currentGitBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func FormatRelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		weeks := int(d.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}
}

func FormatFileSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		kb := float64(bytes) / 1024
		if kb == float64(int(kb)) {
			return fmt.Sprintf("%.0fKB", kb)
		}
		return fmt.Sprintf("%.1fKB", kb)
	default:
		mb := float64(bytes) / 1024 / 1024
		return fmt.Sprintf("%.1fMB", mb)
	}
}

func MatchesSearch(s SessionInfo, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(s.FirstMessage), q) ||
		strings.Contains(strings.ToLower(s.ID), q)
}
