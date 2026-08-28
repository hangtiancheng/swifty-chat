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

package agent_hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty.go/swifty_http"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/mcp"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

const (
	// idleTimeout is how long an agent with no open client and no work may sit
	// before its workspace and context are released. Its MCP connections are
	// shared and stay up.
	idleTimeout = 30 * time.Minute
	sweepEvery  = 5 * time.Minute

	// mcpConnectTimeout caps how long the shared pool waits on MCP servers.
	// Whatever answered by then is used; the rest are reported as errors.
	mcpConnectTimeout = 45 * time.Second

	// A server missing from the pool is retried on later cold starts. The
	// window is shorter than the first attempt, because the pool is already
	// serving and the agent triggering the retry should not wait long for a
	// server that is probably still down. Repeated failures back off from
	// mcpRetryMinInterval up to mcpRetryMaxInterval so a permanently broken
	// server stops being dialled — and stops filling the log — on its own.
	mcpRetryTimeout     = 10 * time.Second
	mcpRetryMinInterval = 1 * time.Minute
	mcpRetryMaxInterval = 30 * time.Minute
)

// Manager owns one agent per user, created on first contact and dropped once
// idle. Configuration is swiftx's own: whatever the operator put in
// ~/.swiftx/config.yaml or .swiftx/config.yaml applies here too, which is how
// providers, MCP servers, hooks and the permission mode arrive without this
// project defining a second set of knobs.
type Manager struct {
	sink ChatSink
	root string

	cfg      *config.AppConfig
	provider config.ProviderConfig
	cfgErr   error

	mu       sync.Mutex
	sessions map[string]*Session

	// MCP servers are process-wide: connecting one means a child process or a
	// socket, so every user's agent shares the same connections instead of
	// starting its own set. Sharing is also visible behaviour — users reach the
	// same server instances, and whatever state those hold is common ground.
	mcpMu           sync.Mutex
	mcpMgr          *mcp.Manager
	mcpInstructions string
	mcpLastTry      time.Time
	mcpRetryIn      time.Duration

	done     chan struct{}
	stopOnce sync.Once
}

func NewManager(sink ChatSink) *Manager {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	m := &Manager{
		sink:     sink,
		root:     filepath.Join(wd, ".swiftx", "chat"),
		sessions: make(map[string]*Session),
		done:     make(chan struct{}),
	}
	cfg, err := config.LoadConfig("")
	switch {
	case err != nil:
		// A server without swiftx configured must still serve chat, so this is
		// recorded and reported per conversation instead of failing startup.
		m.cfgErr = err
		log.Printf("agent_hub: assistant disabled: %v", err)
	default:
		m.cfg = cfg
		m.provider = cfg.Providers[0]
		log.Printf("agent_hub: assistant ready (model %s, workspaces %s)", m.provider.Model, m.root)
	}
	go m.sweepIdle()
	return m
}

// Dispatch hands a persisted chat message to its author's agent. It runs on the
// chat event loop, so it only ever does map work plus a cheap session
// construction; everything slow happens on the session's own goroutine.
func (m *Manager) Dispatch(userID, chatSessionID, messageID, content string) {
	sess, err := m.session(userID)
	if err != nil {
		log.Printf("agent_hub: dispatch for %s failed: %v", userID, err)
		m.sink.SaveAssistantText(userID, chatSessionID, fmt.Sprintf("I can't reply right now — %s", err))
		return
	}
	sess.enqueue(promptJob{chatSessionID: chatSessionID, messageID: messageID, content: content})
}

// Serve runs the control socket for one client: it reports progress for the
// user's agent and carries back the answers that permission and question
// prompts block on.
func (m *Manager) Serve(userID string, ws *swifty_http.WSConn) {
	defer ws.Close()

	sess, err := m.session(userID)
	if err != nil {
		writeEvent(ws, Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return
	}

	sess.attach(ws)
	defer sess.detach(ws)

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			if !errors.Is(err, swifty_http.ErrWSClosed) {
				log.Printf("agent_hub %s: read error: %v", userID, err)
			}
			return
		}
		var frame clientFrame
		if json.Unmarshal(raw, &frame) != nil {
			continue
		}
		switch frame.Type {
		case "permission_response":
			var reply permissionReply
			if json.Unmarshal(frame.Data, &reply) == nil {
				sess.resolvePermission(reply)
			}
		case "ask_user_response":
			var reply askUserReply
			if json.Unmarshal(frame.Data, &reply) == nil {
				sess.resolveAsk(reply)
			}
		case "cancel":
			sess.cancel()
		case "ping":
			writeEvent(ws, Event{Type: "pong"})
		}
	}
}

// session returns the user's agent, building it on first use.
func (m *Manager) session(userID string) (*Session, error) {
	if m.cfgErr != nil {
		return nil, fmt.Errorf("Swiftx is not configured on this server: %w", m.cfgErr)
	}
	if !safeSegment(userID) {
		return nil, fmt.Errorf("invalid user id")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if sess, ok := m.sessions[userID]; ok {
		return sess, nil
	}
	sess, err := newSession(m, userID, filepath.Join(m.root, userID))
	if err != nil {
		return nil, err
	}
	m.sessions[userID] = sess
	return sess, nil
}

// sweepIdle releases agents nobody is using. An agent holds a workspace, a
// context and possibly MCP child processes, so leaving one per user who ever
// said hello would grow without bound.
func (m *Manager) sweepIdle() {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.mu.Lock()
			var evicted []*Session
			for id, sess := range m.sessions {
				streaming, _, _, _ := sess.snapshot()
				if streaming || sess.hasConns() || sess.idleFor() < idleTimeout {
					continue
				}
				delete(m.sessions, id)
				evicted = append(evicted, sess)
			}
			m.mu.Unlock()
			for _, sess := range evicted {
				log.Printf("agent_hub: releasing idle agent for %s", sess.userID)
				sess.close()
			}
		}
	}
}

// sharedMCP hands out the process-wide pool, connecting it on first use.
//
// Later calls retry whatever is still missing: a server that happened to be
// down when the first agent warmed up can join later, which is why connections
// are not established once and forgotten. Successful connections are never
// touched again. All access is serialised here because mcp.Manager keeps plain
// maps of its clients.
func (m *Manager) sharedMCP() (*mcp.Manager, string) {
	if m.cfg == nil || len(m.cfg.MCPServers) == 0 {
		return nil, ""
	}

	m.mcpMu.Lock()
	defer m.mcpMu.Unlock()

	if m.mcpMgr == nil {
		serverConfigs := make([]mcp.ServerConfig, 0, len(m.cfg.MCPServers))
		for _, c := range m.cfg.MCPServers {
			serverConfigs = append(serverConfigs, mcp.ServerConfig{
				Name: c.Name, Command: c.Command, Args: c.Args,
				URL: c.URL, Transport: c.Transport, Headers: c.Headers, Env: c.Env,
			})
		}
		m.mcpMgr = mcp.NewManager()
		m.mcpMgr.LoadConfigs(serverConfigs)
		m.mcpRetryIn = mcpRetryMinInterval
		m.connectMissingLocked(mcpConnectTimeout)
		return m.mcpMgr, m.mcpInstructions
	}

	if len(m.mcpMgr.MissingServers()) > 0 && time.Since(m.mcpLastTry) >= m.mcpRetryIn {
		// Retries run on a tighter clock than the first attempt: the pool is
		// already usable, so a still-dead server must not stall the agent that
		// happens to trigger the retry.
		m.connectMissingLocked(mcpRetryTimeout)
	}
	return m.mcpMgr, m.mcpInstructions
}

// connectMissingLocked runs one connect pass over the servers that are still
// down. Callers must hold mcpMu.
func (m *Manager) connectMissingLocked(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	m.mcpLastTry = time.Now()
	result := m.mcpMgr.ConnectAll(ctx)
	for _, errMsg := range result.Errors {
		log.Printf("agent_hub: %s — its tools are unavailable, the rest keep working", errMsg)
	}

	missing := m.mcpMgr.MissingServers()
	if len(result.Servers) > 0 {
		m.mcpInstructions = buildMCPInstructions(m.mcpMgr.ConnectedServers(), m.mcpMgr.NewToolSet())
		// Progress resets the backoff: whatever was wrong may be clearing up.
		m.mcpRetryIn = mcpRetryMinInterval
		log.Printf("agent_hub: MCP pool has %d server(s) connected, %d still missing%s",
			len(m.mcpMgr.ConnectedServers()), len(missing), formatMissing(missing))
		return
	}
	if len(missing) == 0 {
		return
	}
	// Nothing new came up, so back off before trying again. Otherwise a
	// permanently broken server would be dialled on every cold start forever.
	m.mcpRetryIn = min(m.mcpRetryIn*2, mcpRetryMaxInterval)
	log.Printf("agent_hub: %d MCP server(s) still unavailable%s, next retry in %s",
		len(missing), formatMissing(missing), m.mcpRetryIn)
}

func formatMissing(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	return " (" + strings.Join(missing, ", ") + ")"
}

// buildMCPInstructions describes the connected servers for the model. It is the
// same text for everyone, so it is rebuilt with the pool rather than per agent.
func buildMCPInstructions(servers []mcp.ServerInfo, toolSet []tools.Tool) string {
	if len(servers) == 0 {
		return ""
	}
	toolsByServer := make(map[string][]string)
	for _, t := range toolSet {
		name := t.Name()
		for _, srv := range servers {
			if strings.HasPrefix(name, mcp.MCPToolNamePrefix(srv.Name)) {
				toolsByServer[srv.Name] = append(toolsByServer[srv.Name], name)
				break
			}
		}
	}
	var parts []string
	for _, srv := range servers {
		var sb strings.Builder
		fmt.Fprintf(&sb, "## %s\n", srv.Name)
		if srv.Instructions != "" {
			fmt.Fprintf(&sb, "%s\n", srv.Instructions)
		}
		if names, ok := toolsByServer[srv.Name]; ok && len(names) > 0 {
			fmt.Fprintf(&sb, "\nAvailable tools: %s", strings.Join(names, ", "))
		}
		parts = append(parts, sb.String())
	}
	return "# MCP Server Instructions\n\nThe following MCP servers are connected. Use their tools when the user asks.\n\n" +
		strings.Join(parts, "\n\n")
}

func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.done)
		m.mu.Lock()
		sessions := make([]*Session, 0, len(m.sessions))
		for _, sess := range m.sessions {
			sessions = append(sessions, sess)
		}
		m.sessions = make(map[string]*Session)
		m.mu.Unlock()
		for _, sess := range sessions {
			sess.close()
		}
		// The pool outlives individual agents, so it is closed here and nowhere
		// else: a single session shutting it down would cut off everyone.
		if m.mcpMgr != nil {
			m.mcpMgr.Shutdown()
		}
	})
}

func writeEvent(ws *swifty_http.WSConn, ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_ = ws.WriteMessage(swifty_http.TextMessage, data)
}

// safeSegment reports whether id can be trusted as a single path segment. The
// id comes from a token claim and becomes a directory name, so anything that
// could climb out of the workspace root is refused rather than sanitized.
func safeSegment(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
