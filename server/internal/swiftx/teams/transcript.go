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

package teams

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

// transcriptEntry is a single conversation record serialized to disk.
type transcriptEntry struct {
	Role        string                 `json:"role"`
	Content     string                 `json:"content,omitempty"`
	ToolUses    []transcriptToolUse    `json:"tool_uses,omitempty"`
	ToolResults []transcriptToolResult `json:"tool_results,omitempty"`
}

type transcriptToolUse struct {
	ToolUseID string         `json:"tool_use_id"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type transcriptToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// serializeConversation serializes conversation history into a persistable JSON format.
func serializeConversation(conv *conversation.Manager) []transcriptEntry {
	var entries []transcriptEntry
	for _, msg := range conv.GetMessages() {
		entry := transcriptEntry{Role: msg.Role, Content: msg.Content}
		for _, tu := range msg.ToolUses {
			entry.ToolUses = append(entry.ToolUses, transcriptToolUse{
				ToolUseID: tu.ToolUseID,
				ToolName:  tu.ToolName,
				Arguments: tu.Arguments,
			})
		}
		for _, tr := range msg.ToolResults {
			entry.ToolResults = append(entry.ToolResults, transcriptToolResult{
				ToolUseID: tr.ToolUseID,
				Content:   tr.Content,
				IsError:   tr.IsError,
			})
		}
		entries = append(entries, entry)
	}
	return entries
}

// deserializeConversation restores a conversation manager from disk format.
func deserializeConversation(entries []transcriptEntry) *conversation.Manager {
	conv := conversation.NewManager()
	for _, e := range entries {
		var toolUses []conversation.ToolUseBlock
		for _, tu := range e.ToolUses {
			toolUses = append(toolUses, conversation.ToolUseBlock{
				ToolUseID: tu.ToolUseID,
				ToolName:  tu.ToolName,
				Arguments: tu.Arguments,
			})
		}
		var toolResults []conversation.ToolResultBlock
		for _, tr := range e.ToolResults {
			toolResults = append(toolResults, conversation.ToolResultBlock{
				ToolUseID: tr.ToolUseID,
				Content:   tr.Content,
				IsError:   tr.IsError,
			})
		}
		msg := conversation.Message{
			Role:        e.Role,
			Content:     e.Content,
			ToolUses:    toolUses,
			ToolResults: toolResults,
		}
		conv.AppendMessages([]conversation.Message{msg})
	}
	return conv
}

// transcriptDir returns the transcript storage directory for a team.
func transcriptDir(teamName string) string {
	return filepath.Join(teamsBaseDir(), teamName, "transcripts")
}

// SaveTranscript persists a teammate's conversation history to disk for debugging and troubleshooting.
// File path is .swiftx/teams/<team>/transcripts/<agentID>.json.
func SaveTranscript(teamName, agentID string, conv *conversation.Manager) (string, error) {
	dir := transcriptDir(teamName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, agentID+".json")
	data, err := json.MarshalIndent(serializeConversation(conv), "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

// LoadTranscript loads a teammate's conversation history from disk.
// Returns nil if the file does not exist or parsing fails.
func LoadTranscript(teamName, agentID string) *conversation.Manager {
	path := filepath.Join(transcriptDir(teamName), agentID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []transcriptEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	return deserializeConversation(entries)
}
