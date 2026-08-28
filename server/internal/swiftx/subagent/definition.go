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

package subagent

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentMemoryScope Persistent memory location: per-user, per-project, or per-checkout (not version
// controlled).
type AgentMemoryScope string

const (
	AgentMemoryScopeUser    AgentMemoryScope = "user"
	AgentMemoryScopeProject AgentMemoryScope = "project"
	AgentMemoryScopeLocal   AgentMemoryScope = "local"
)

// IsolationMode encodes the `isolation` frontmatter field.
type IsolationMode string

const (
	IsolationWorktree IsolationMode = "worktree"
	IsolationRemote   IsolationMode = "remote"
)

// AgentDefinition Fields with no runtime usage yet (Effort, Skills, McpServers, Hooks, Memory,
// InitialPrompt, OmitSwiftxMd, RequiredMcpServers) are still parsed so user definitions don't lose
// data on the round-trip and so future channels can pick them up without another schema migration.
type AgentDefinition struct {
	AgentType       string   `yaml:"name"`
	WhenToUse       string   `yaml:"description"`
	Tools           []string `yaml:"tools"`
	DisallowedTools []string `yaml:"disallowedTools"`
	Model           string   `yaml:"model"`
	MaxTurns        int      `yaml:"maxTurns"`

	// permissionMode overrides the parent agent's permission mode for this sub-agent. Valid values
	// match internal/permissions.PermissionMode.
	PermissionMode string `yaml:"permissionMode"`

	// Effort is a hint to the model about task complexity ("low" | "medium" | "high" | int). Currently
	// stored only, not yet consumed.
	Effort any `yaml:"effort"`

	// Skills are skill names to preload when the sub-agent starts.
	Skills []string `yaml:"skills"`

	// McpServers are MCP server names or inline configs scoped to this agent. Stored as raw any so
	// future loading can interpret either string refs or inline configs.
	McpServers []any `yaml:"mcpServers"`

	// RequiredMcpServers gates the agent: if listed servers aren't available at load time, the agent
	// is filtered out by hasRequiredMcpServers.
	RequiredMcpServers []string `yaml:"requiredMcpServers"`

	// Hooks are session-scoped hooks registered when this agent starts. Stored as raw YAML; the hooks
	// package will type-check on consumption.
	Hooks any `yaml:"hooks"`

	// Memory enables persistent memory in one of three scopes.
	Memory AgentMemoryScope `yaml:"memory"`

	// Background forces this agent to always run as a background task when spawned, regardless of
	// run_in_background parameter.
	Background bool `yaml:"background"`

	// Isolation selects a file-system isolation mode for the spawn.
	Isolation IsolationMode `yaml:"isolation"`

	// InitialPrompt is prepended to the first user turn (slash commands work).
	InitialPrompt string `yaml:"initialPrompt"`

	// OmitSwiftxMd drops the SWIFTX.md hierarchy from this agent's user context. Read-only agents
	// (Explore, Plan) save tokens by skipping it.
	OmitSwiftxMd bool `yaml:"omitSwiftxMd"`

	// SystemPrompt is the Markdown body of the definition file.
	SystemPrompt string `yaml:"-"`

	// FilePath / Source / Filename are populated at load time.
	FilePath string `yaml:"-"`
	Source   string `yaml:"-"`
	Filename string `yaml:"-"`
}

// validPermissionModes lists the legal values for the permission_mode field in
// an agent definition; the empty string means "no override" — inherit the
// parent agent's mode.
var validPermissionModes = map[string]bool{
	"":                  true,
	"acceptEdits":       true,
	"bypassPermissions": true,
	"default":           true,
	"plan":              true,
}

var validMemoryScopes = map[AgentMemoryScope]bool{
	"":                      true,
	AgentMemoryScopeUser:    true,
	AgentMemoryScopeProject: true,
	AgentMemoryScopeLocal:   true,
}

var validIsolationModes = map[IsolationMode]bool{
	"":                true,
	IsolationWorktree: true,
	IsolationRemote:   true,
}

func ParseAgentFile(path string) (*AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	var def AgentDefinition
	def.FilePath = path

	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			if err := yaml.Unmarshal([]byte(parts[1]), &def); err != nil {
				return nil, fmt.Errorf("parse frontmatter in %s: %w", path, err)
			}
			def.SystemPrompt = strings.TrimSpace(parts[2])
		}
	} else {
		def.SystemPrompt = strings.TrimSpace(content)
	}

	if def.AgentType == "" {
		return nil, fmt.Errorf("agent definition %s: missing required field 'name'", path)
	}
	if def.WhenToUse == "" {
		return nil, fmt.Errorf("agent definition %s: missing required field 'description'", path)
	}

	// Normalize and validate `model`. Matches AgentJsonSchema: only "must be a non-empty string" —
	// actual availability is left to the host's ModelResolver / LLM router. Third-party model names
	// like "glm-5.1" must round-trip. Lowercase "inherit" normalizes to "inherit" (the sentinel that
	// means "use parent's client"); everything else stays verbatim so the router can match.
	def.Model = strings.TrimSpace(def.Model)
	if strings.EqualFold(def.Model, "inherit") {
		def.Model = "inherit"
	}

	if !validPermissionModes[def.PermissionMode] {
		return nil, fmt.Errorf("agent definition %s: invalid permissionMode '%s'", path, def.PermissionMode)
	}

	if !validMemoryScopes[def.Memory] {
		return nil, fmt.Errorf("agent definition %s: invalid memory scope '%s'", path, def.Memory)
	}

	if !validIsolationModes[def.Isolation] {
		return nil, fmt.Errorf("agent definition %s: invalid isolation mode '%s'", path, def.Isolation)
	}

	return &def, nil
}

func (d *AgentDefinition) ToSpec() SubAgentSpec {
	return SubAgentSpec{
		Name:                 d.AgentType,
		Description:          d.WhenToUse,
		Tools:                d.Tools,
		DisallowedTools:      d.DisallowedTools,
		SystemPromptOverride: d.SystemPrompt,
		MaxTurns:             d.MaxTurns,
		Model:                d.Model,
		PermissionMode:       d.PermissionMode,
		Background:           d.Background,
		Isolation:            d.Isolation,
		InitialPrompt:        d.InitialPrompt,
		OmitSwiftxMd:         d.OmitSwiftxMd,
		Skills:               d.Skills,
		Memory:               d.Memory,
		McpServers:           d.McpServers,
		RequiredMcpServers:   d.RequiredMcpServers,
		Hooks:                d.Hooks,
		Effort:               d.Effort,
	}
}

// HasRequiredMcpServers Returns true when the agent has no MCP requirements or every required
// pattern matches an available server name (case-insensitive substring).
func (d *AgentDefinition) HasRequiredMcpServers(availableServers []string) bool {
	if len(d.RequiredMcpServers) == 0 {
		return true
	}
	for _, pattern := range d.RequiredMcpServers {
		patLower := strings.ToLower(pattern)
		matched := false
		for _, server := range availableServers {
			if strings.Contains(strings.ToLower(server), patLower) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
