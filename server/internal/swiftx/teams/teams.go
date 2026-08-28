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
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

type TeamMode string

const (
	ModeInProcess TeamMode = "in-process"
	ModeTmux      TeamMode = "tmux"
)

// teamsBaseDir is the root directory for all team directories. It lives under
// the user's home directory rather than the project directory, because pane
// teammates are independent processes whose working directory may be changed
// by worktrees; using the home directory ensures both teammate processes and
// the Lead find the same team configuration.
func teamsBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		wd, _ := os.Getwd()
		home = wd
	}
	return filepath.Join(home, ".swiftx", "teams")
}

type Member struct {
	Name     string
	AgentRef *agent.Agent
	Conv     *conversation.Manager
	Active   bool
	Cancel   context.CancelFunc
	// PaneID is the backend-specific handle assigned by tmux/iTerm
	// spawn (e.g. window or tab name). Empty for in-process members.
	PaneID   string
	Progress *TeammateProgress

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
	Mode    TeamMode
	Members map[string]*Member
	MailBox *FileMailBox
	mu      sync.Mutex

	// Team-level metadata for persistence.
	LeadAgentID string
	Description string
	CreatedAt   int64
}

func NewTeam(name string, mode TeamMode) *Team {
	inboxDir := filepath.Join(teamDir(name), "inboxes")
	return &Team{
		Name:      name,
		Mode:      mode,
		Members:   make(map[string]*Member),
		MailBox:   NewFileMailBox(inboxDir),
		CreatedAt: time.Now().Unix(),
	}
}

func (t *Team) AddMember(name string, client llm.Client, registry *tools.Registry, protocol string) *Member {
	t.mu.Lock()
	defer t.mu.Unlock()

	ag := agent.New(client, registry, protocol)
	member := &Member{
		Name:     name,
		AgentRef: ag,
		Conv:     conversation.NewManager(),
		Active:   false,
		AgentID:  name,
		JoinedAt: time.Now().Unix(),
	}
	t.Members[name] = member
	t.persist()
	return member
}

// SetMemberMeta fills in member metadata (agent type, model, worktree path)
// and persists it. The spawn flow obtains this information later than
// AddMember, so it is written in two steps.
func (t *Team) SetMemberMeta(name, agentType, model, worktreePath string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	member, ok := t.Members[name]
	if !ok {
		return
	}
	member.AgentType = agentType
	member.Model = model
	member.WorktreePath = worktreePath
	t.persist()
}

func (t *Team) StartMember(ctx context.Context, name string, task string) (<-chan agent.AgentEvent, error) {
	t.mu.Lock()
	member, ok := t.Members[name]
	t.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("member not found: %s", name)
	}

	memberCtx, cancel := context.WithCancel(ctx)
	member.Active = true
	member.Cancel = cancel

	t.mu.Lock()
	t.persist()
	t.mu.Unlock()

	member.Conv.AddUserMessage(task)
	ch := member.AgentRef.Run(memberCtx, member.Conv)
	return ch, nil
}

func (t *Team) StopMember(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	member, ok := t.Members[name]
	if !ok {
		return
	}
	// External backends (tmux/iTerm) own a real OS pane that must be
	// torn down before clearing the local handle. In-process members
	// just need the goroutine cancelled.
	if member.PaneID != "" {
		switch t.Mode {
		case ModeTmux:
			stopTmuxTeammate(member.PaneID)
		case ModeITerm:
			stopITermTeammate(member.PaneID)
		}
	}
	if member.Cancel != nil {
		member.Cancel()
	}
	member.Active = false
	t.persist()
}

func (t *Team) GetTeammateProgress() []*TeammateProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	var result []*TeammateProgress
	for _, m := range t.Members {
		if m.Progress != nil {
			result = append(result, m.Progress)
		}
	}
	return result
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

func (tm *TeamManager) CreateTeam(name string, mode TeamMode) *Team {
	return tm.CreateTeamFull(name, mode, "", "")
}

// CreateTeamFull creates a team, records the lead and description, then writes
// the configuration to config.json. Once persisted, teammate processes and
// future sessions can recover the team via GetTeam.
func (tm *TeamManager) CreateTeamFull(name string, mode TeamMode, leadAgentID, description string) *Team {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	team := NewTeam(name, mode)
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

// CreateTeamWith registers an externally-constructed Team. Worker
// processes spawned by tmux/iTerm build a Team locally (pointing at
// the same mailbox dir as the lead's) and use this to expose it to
// SendMessage in the same process.
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
	team := NewTeam(tf.Name, ModeInProcess)
	team.LeadAgentID = tf.LeadAgentID
	team.Description = tf.Description
	team.CreatedAt = tf.CreatedAt
	for _, m := range tf.Members {
		if m.BackendType != "" {
			team.Mode = TeamMode(m.BackendType)
		}
		active := false
		if m.IsActive != nil {
			active = *m.IsActive
		}
		team.Members[m.Name] = &Member{
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
		for memberName := range team.Members {
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

func (tm *TeamManager) GetAllTeammateProgress() []*TeammateProgress {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	var result []*TeammateProgress
	for _, team := range tm.teams {
		result = append(result, team.GetTeammateProgress()...)
	}
	return result
}

func (tm *TeamManager) CloseAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for name, team := range tm.teams {
		for memberName := range team.Members {
			team.StopMember(memberName)
		}
		delete(tm.teams, name)
	}
}
