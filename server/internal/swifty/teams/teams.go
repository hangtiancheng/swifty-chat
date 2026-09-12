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
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/tools"
)

// teamsBaseDir is the root directory for all team directories. It lives under
// the user's home directory rather than the project directory so the team
// configuration survives worktree switches and server restarts.
func teamsBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		wd, _ := os.Getwd()
		home = wd
	}
	return filepath.Join(home, ".swifty", "teams")
}

type Member struct {
	Name     string
	AgentRef *agent.Agent
	Conv     *conversation.Manager
	Active   bool
	Cancel   context.CancelFunc

	// The following fields are metadata for persistence; they do not
	// participate in runtime scheduling and are only used when writing
	// config.json and restoring a team from disk.
	AgentID      string
	AgentType    string
	Model        string
	WorktreePath string
	JoinedAt     int64
}

type Team struct {
	Name    string
	MailBox *FileMailBox

	// members is guarded by mu. Use the HasMember/GetMember/MemberNames
	// accessors instead of touching the map directly.
	members map[string]*Member
	mu      sync.Mutex

	// Team-level metadata for persistence.
	LeadAgentID string
	Description string
	CreatedAt   int64
}

func NewTeam(name string) *Team {
	inboxDir := filepath.Join(teamDir(name), "inboxes")
	return &Team{
		Name:      name,
		members:   make(map[string]*Member),
		MailBox:   NewFileMailBox(inboxDir),
		CreatedAt: time.Now().Unix(),
	}
}

// MemberInit carries everything AddMember needs to build a fully configured
// member in one step. Applying metadata here (instead of patching it in after
// the member goroutine already runs) guarantees the agent's workdir and
// permission checker are in place before its first turn.
type MemberInit struct {
	Client       llm.Client
	Registry     *tools.Registry
	Protocol     string
	Checker      *permissions.Checker
	AgentType    string
	Model        string
	WorktreePath string
}

func (t *Team) AddMember(name string, init MemberInit) *Member {
	t.mu.Lock()
	defer t.mu.Unlock()

	ag := agent.New(init.Client, init.Registry, init.Protocol)
	if init.WorktreePath != "" {
		ag.WorkDir = init.WorktreePath
	}
	ag.Checker = init.Checker
	member := &Member{
		Name:         name,
		AgentRef:     ag,
		Conv:         conversation.NewManager(),
		Active:       false,
		AgentID:      name,
		AgentType:    init.AgentType,
		Model:        init.Model,
		WorktreePath: init.WorktreePath,
		JoinedAt:     time.Now().Unix(),
	}
	t.members[name] = member
	t.persist()
	return member
}

func (t *Team) StopMember(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	member, ok := t.members[name]
	if !ok {
		return
	}
	if member.Cancel != nil {
		member.Cancel()
	}
	member.Active = false
	t.persist()
}

// HasMember reports whether name is registered on the team.
func (t *Team) HasMember(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.members[name]
	return ok
}

// GetMember returns the named member, or nil when absent.
func (t *Team) GetMember(name string) *Member {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.members[name]
}

// MemberNames returns a snapshot of the current member names.
func (t *Team) MemberNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	names := make([]string, 0, len(t.members))
	for n := range t.members {
		names = append(names, n)
	}
	return names
}

// IsMemberActive reports whether the named member exists and is running.
func (t *Team) IsMemberActive(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	member, ok := t.members[name]
	return ok && member.Active
}

func (t *Team) SendMessage(from, to, content string) {
	t.MailBox.Send(to, FileMailMessage{
		From:      from,
		Text:      content,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

type TeamManager struct {
	mu         sync.Mutex
	teams      map[string]*Team
	taskStores map[string]*SharedTaskStore // one shared task store per team
}

func NewTeamManager() *TeamManager {
	return &TeamManager{
		teams:      make(map[string]*Team),
		taskStores: make(map[string]*SharedTaskStore),
	}
}

func teamDir(name string) string {
	return filepath.Join(teamsBaseDir(), sanitizeTeamName(name))
}

func (tm *TeamManager) CreateTeam(name string) *Team {
	return tm.CreateTeamFull(name, "", "")
}

// CreateTeamFull creates a team, records the lead and description, then writes
// the configuration to config.json. Once persisted, future sessions can
// recover the team via GetTeam.
func (tm *TeamManager) CreateTeamFull(name string, leadAgentID, description string) *Team {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	team := NewTeam(name)
	team.LeadAgentID = leadAgentID
	team.Description = description
	tm.teams[name] = team
	// Initialize an empty shared task store for the new team.
	store := NewSharedTaskStore(filepath.Join(teamDir(name), "tasks.json"))
	store.InitEmpty()
	tm.taskStores[name] = store
	team.persist()
	return team
}

// GetTaskStore returns the team's shared task store; when not cached in memory
// (e.g. in a teammate process), it loads from tasks.json on disk.
func (tm *TeamManager) GetTaskStore(teamName string) *SharedTaskStore {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if store, ok := tm.taskStores[teamName]; ok {
		return store
	}
	store := NewSharedTaskStore(filepath.Join(teamDir(teamName), "tasks.json"))
	tm.taskStores[teamName] = store
	return store
}

// CreateTeamWith registers an externally-constructed Team so SendMessage
// and the coordination tools can reach it in this process.
func (tm *TeamManager) CreateTeamWith(team *Team) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.teams[team.Name] = team
}

// GetTeam checks memory first; on miss, looks for config.json on disk.
// A Team reconstructed from disk carries only metadata — member agent
// instances and conversations are empty — sufficient for SendMessage to
// deliver by name and for UI display; actually running a member requires
// re-spawning.
func (tm *TeamManager) GetTeam(name string) *Team {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if team, ok := tm.teams[name]; ok {
		return team
	}
	tf, err := ReadTeamFile(name)
	if err != nil || tf == nil {
		return nil
	}
	team := NewTeam(tf.Name)
	team.LeadAgentID = tf.LeadAgentID
	team.Description = tf.Description
	team.CreatedAt = tf.CreatedAt
	for _, m := range tf.Members {
		active := false
		if m.IsActive != nil {
			active = *m.IsActive
		}
		team.members[m.Name] = &Member{
			Name:         m.Name,
			AgentID:      m.AgentID,
			AgentType:    m.AgentType,
			Model:        m.Model,
			WorktreePath: m.WorktreePath,
			JoinedAt:     m.JoinedAt,
			Active:       active,
		}
	}
	tm.teams[name] = team
	return team
}

func (tm *TeamManager) DeleteTeam(name string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if team, ok := tm.teams[name]; ok {
		registry := GetNameRegistry()
		for _, memberName := range team.MemberNames() {
			team.StopMember(memberName)
			// Unbind this member's mapping in the global name registry.
			registry.Unregister(memberName)
		}
		delete(tm.teams, name)
	}
	delete(tm.taskStores, name)
	// The team directory contains config.json, tasks.json, and inboxes; once
	// the team is gone, remove them all to prevent a future same-named team
	// from picking up stale data.
	_ = os.RemoveAll(teamDir(name))
}

func (tm *TeamManager) ListTeams() []string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	var names []string
	for name := range tm.teams {
		names = append(names, name)
	}
	return names
}

// CloseAll stops every member of every team. Session.close calls this so
// teammate goroutines cannot outlive the session that spawned them.
func (tm *TeamManager) CloseAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for name, team := range tm.teams {
		for _, memberName := range team.MemberNames() {
			team.StopMember(memberName)
		}
		delete(tm.teams, name)
	}
}
