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
	"regexp"
	"strings"
	"time"
)

// TeamFile is the on-disk representation of team configuration, stored at
// <teamsBaseDir>/<slug>/config.json.
//
// The in-memory Member holds agent instances, conversations, and cancel
// functions — none of which are serializable — so the persisted form is a
// separate metadata-only structure. The two are correlated by member name.
//
// This file solves cross-process and cross-restart concerns: pane teammates
// are independent processes that need to know which team they belong to and
// who their peers are; users restarting Swiftx must be able to resume
// previously created teams.
type TeamFile struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	CreatedAt   int64            `json:"createdAt"`
	LeadAgentID string           `json:"leadAgentId"`
	Members     []TeamMemberFile `json:"members"`
}

// TeamMemberFile is the metadata for a single member. IsActive uses a pointer
// to distinguish three states: nil means just registered and not yet started,
// true means running, false means idle.
type TeamMemberFile struct {
	AgentID      string `json:"agentId"`
	Name         string `json:"name"`
	AgentType    string `json:"agentType,omitempty"`
	Model        string `json:"model,omitempty"`
	JoinedAt     int64  `json:"joinedAt"`
	WorktreePath string `json:"worktreePath,omitempty"`
	BackendType  string `json:"backendType,omitempty"`
	IsActive     *bool  `json:"isActive,omitempty"`
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// sanitizeTeamName compresses a team name into a form usable as a directory
// name: all non-alphanumeric characters are replaced with hyphens and
// lowercased. Team names are chosen by the LLM and may contain spaces,
// non-ASCII characters, and punctuation; without sanitization, various
// filesystem issues would arise.
func sanitizeTeamName(name string) string {
	return strings.ToLower(nonAlnum.ReplaceAllString(name, "-"))
}

func teamFilePath(name string) string {
	return filepath.Join(teamDir(name), "config.json")
}

// ReadTeamFile reads team configuration. Returns (nil, nil) when the file does
// not exist, allowing the caller to treat it as "no such team" rather than
// propagating an error.
func ReadTeamFile(name string) (*TeamFile, error) {
	data, err := os.ReadFile(teamFilePath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tf TeamFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, err
	}
	return &tf, nil
}

// WriteTeamFile writes team configuration, creating the directory if needed.
func WriteTeamFile(name string, tf *TeamFile) error {
	dir := teamDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(teamFilePath(name), data, 0o644)
}

// snapshot exports the in-memory Team into a persistable TeamFile.
// The caller must hold t.mu.
func (t *Team) snapshot() *TeamFile {
	tf := &TeamFile{
		Name:        t.Name,
		Description: t.Description,
		CreatedAt:   t.CreatedAt,
		LeadAgentID: t.LeadAgentID,
		Members:     make([]TeamMemberFile, 0, len(t.Members)),
	}
	if tf.CreatedAt == 0 {
		tf.CreatedAt = time.Now().Unix()
	}
	for _, m := range t.Members {
		active := m.Active
		tf.Members = append(tf.Members, TeamMemberFile{
			AgentID:      m.AgentID,
			Name:         m.Name,
			AgentType:    m.AgentType,
			Model:        m.Model,
			JoinedAt:     m.JoinedAt,
			WorktreePath: m.WorktreePath,
			BackendType:  string(t.Mode),
			IsActive:     &active,
		})
	}
	return tf
}

// persist writes the current state back to disk. Write failures do not affect
// the in-memory team's continued operation, so errors are swallowed here:
// persistence serves cross-process and cross-restart needs, not runtime
// correctness. The caller must hold t.mu.
func (t *Team) persist() {
	_ = WriteTeamFile(t.Name, t.snapshot())
}
