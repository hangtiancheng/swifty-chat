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

	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/agent"
	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/permissions"
	"github.com/hangtiancheng/swifty-chat/server/internal/swifty/tools"
)

// TeammateSpawnConfig collects every parameter SpawnTeammate needs.
// Team / MemberName / Task / Addendum: always used.
// Client / Registry / Protocol: handed to the in-process member's agent.
// Workdir: optional working directory override. When non-empty, the
// member's Agent.WorkDir is pointed there. Used for worktree isolation
// so concurrent teammates don't fight over files.
// AgentType / Model: persistence metadata recorded in config.json.
type TeammateSpawnConfig struct {
	Team       *Team
	MemberName string
	Task       string
	Addendum   string

	Client   llm.Client
	Registry *tools.Registry
	Protocol string

	Workdir   string
	AgentType string
	Model     string

	// Checker is the teammate's permission checker. When the Lead marks
	// plan_mode_required during dispatch, this is a ModePlan checker — the
	// teammate can only read, not modify, until the plan is approved.
	Checker *permissions.Checker
}

// SpawnTeammate creates a new team member and launches it as an in-process
// goroutine. It is the single entry point used by the Agent tool's team_name
// code path.
//
// The returned channel forwards every agent event the member emits; it closes
// when the member's loop exits.
func SpawnTeammate(ctx context.Context, cfg TeammateSpawnConfig) (<-chan agent.AgentEvent, error) {
	if cfg.Team == nil {
		return nil, fmt.Errorf("SpawnTeammate: team is required")
	}
	if cfg.MemberName == "" {
		return nil, fmt.Errorf("SpawnTeammate: member name is required")
	}

	// Register the member name in the global name registry so SendMessage can
	// resolve and deliver by name.
	GetNameRegistry().Register(cfg.MemberName, cfg.MemberName)

	return StartInProcessMember(ctx, cfg), nil
}
