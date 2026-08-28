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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/hooks"
)

var envKeyMap = map[string]string{
	"anthropic":     "ANTHROPIC_API_KEY",
	"openai":        "OPENAI_API_KEY",
	"openai-compat": "OPENAI_API_KEY",
}

var validProtocols = map[string]bool{
	"anthropic":     true,
	"openai":        true,
	"openai-compat": true,
}

type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string { return e.Message }

type ProviderConfig struct {
	Name            string `yaml:"name"`
	Protocol        string `yaml:"protocol"`
	BaseURL         string `yaml:"base_url"`
	Model           string `yaml:"model"`
	APIKey          string `yaml:"api_key"`
	Thinking        bool   `yaml:"thinking"`
	ContextWindow   int    `yaml:"context_window"`
	MaxOutputTokens int    `yaml:"max_output_tokens"`

	// fetchedContextWindow caches the max_input_tokens auto-pulled from the
	// provider's /v1/models endpoint (layer 2 of GetContextWindow). Populated
	// once at client init via SetFetchedContextWindow; 0 means "not fetched".
	// Not a yaml field — it's a runtime cache, never persisted.
	fetchedContextWindow int
}

// modelContextWindows maps a model-name substring to its context window
// (max input tokens). Matched from most specific to most generic; the first
// substring hit wins. Values are reasonable starting points only — they may
// drift as models are updated/renamed. When a value is wrong, set
// `context_window` in config to override (that takes top priority).
var modelContextWindows = []struct {
	substr string
	window int
}{
	{"1m", 1000000},      // also covers "-1m" suffixes (e.g. claude-...-1m)
	{"gpt-4.1", 1000000}, // GPT-4.1 family ships a 1M window
	{"gpt-4o", 128000},
	{"gpt-4-turbo", 128000},
	{"o1", 200000}, // OpenAI reasoning models o1 / o3 / o4
	{"o3", 200000},
	{"o4", 200000},
	{"gpt-3.5", 16385},
	{"claude", 200000},
}

// SetFetchedContextWindow records the context window auto-pulled from the
// provider (layer 2). A non-positive value is ignored so a failed fetch never
// pollutes the cache. Called once per provider at client init.
func (p *ProviderConfig) SetFetchedContextWindow(window int) {
	if window > 0 {
		p.fetchedContextWindow = window
	}
}

// lookupModelContextWindow returns the built-in mapping-table window for the
// given model via substring match (layer 3), or 0 if nothing matches.
func lookupModelContextWindow(model string) int {
	m := strings.ToLower(model)
	for _, e := range modelContextWindows {
		if strings.Contains(m, e.substr) {
			return e.window
		}
	}
	return 0
}

// GetContextWindow resolves the model's context window with four layers of
// fallback, highest priority first:
//
//  1. config-supplied context_window (> 0) — explicit override, always wins.
//  2. value auto-fetched from the provider's /v1/models endpoint, cached via
//     SetFetchedContextWindow (only Anthropic-protocol providers ever set it;
//     a failed/absent fetch leaves it 0 and is skipped).
//  3. built-in model-name → window mapping table (substring match).
//  4. conservative default (claude → 200000, otherwise → 128000).
func (p *ProviderConfig) GetContextWindow() int {
	if p.ContextWindow > 0 {
		return p.ContextWindow
	}
	if p.fetchedContextWindow > 0 {
		return p.fetchedContextWindow
	}
	if w := lookupModelContextWindow(p.Model); w > 0 {
		return w
	}
	if strings.Contains(p.Model, "claude") {
		return 200000
	}
	return 128000
}

func (p *ProviderConfig) GetMaxOutputTokens() int {
	if p.MaxOutputTokens > 0 {
		return p.MaxOutputTokens
	}
	if p.Thinking {
		return 64000
	}
	return 8192
}

func (p *ProviderConfig) ResolveAPIKey() string {
	if p.APIKey != "" {
		return p.APIKey
	}
	envVar := envKeyMap[p.Protocol]
	if envVar == "" {
		return ""
	}
	return os.Getenv(envVar)
}

type MCPServerConfig struct {
	Name      string            `yaml:"name"`
	Command   string            `yaml:"command"`
	Args      []string          `yaml:"args"`
	URL       string            `yaml:"url"`
	Transport string            `yaml:"transport"`
	Headers   map[string]string `yaml:"headers"`
	Env       map[string]string `yaml:"env"`
}

// SandboxConfig controls the OS-level sandbox configuration.
type SandboxConfig struct {
	Enabled        bool `yaml:"enabled"`         // whether the sandbox is enabled
	AutoAllow      bool `yaml:"auto_allow"`      // whether commands inside the sandbox are auto-approved
	NetworkEnabled bool `yaml:"network_enabled"` // whether network access is permitted
}

type AppConfig struct {
	Providers             []ProviderConfig  `yaml:"providers"`
	PermissionMode        string            `yaml:"permission_mode"`
	MCPServers            []MCPServerConfig `yaml:"mcp_servers"`
	Hooks                 []hooks.Hook      `yaml:"hooks"`
	Sandbox               SandboxConfig     `yaml:"sandbox"`
	EnableCoordinatorMode bool              `yaml:"enable_coordinator_mode"`

	// EnableFork controls whether fork is used when subagent_type is omitted.
	// It is a pointer because it defaults to enabled: a plain bool cannot
	// distinguish "not set in config" from "explicitly set to false", and the
	// latter would otherwise be impossible to turn off.
	EnableFork *bool `yaml:"enable_fork"`
}

// ForkEnabled reports whether fork is available. It defaults to enabled when
// the config does not specify a value.
func (c *AppConfig) ForkEnabled() bool {
	return c.EnableFork == nil || *c.EnableFork
}

func loadSingleFile(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Failed to parse config %s: %s", path, err)}
	}
	return &cfg, nil
}

func mergeConfig(base, override *AppConfig) *AppConfig {
	if override.Providers != nil {
		base.Providers = override.Providers
	}
	if override.PermissionMode != "" {
		base.PermissionMode = override.PermissionMode
	}
	if len(override.MCPServers) > 0 {
		byName := make(map[string]int)
		for i, s := range base.MCPServers {
			byName[s.Name] = i
		}
		for _, s := range override.MCPServers {
			if idx, ok := byName[s.Name]; ok {
				base.MCPServers[idx] = s
			} else {
				base.MCPServers = append(base.MCPServers, s)
				byName[s.Name] = len(base.MCPServers) - 1
			}
		}
	}
	base.Hooks = append(base.Hooks, override.Hooks...)
	// Sandbox config: a later-loaded config overrides earlier ones.
	if override.Sandbox.Enabled {
		base.Sandbox = override.Sandbox
	}
	if override.EnableCoordinatorMode {
		base.EnableCoordinatorMode = true
	}
	if override.EnableFork != nil {
		base.EnableFork = override.EnableFork
	}
	return base
}

func validateProviders(cfg *AppConfig) error {
	if len(cfg.Providers) == 0 {
		return &ConfigError{Message: "At least one provider must be configured"}
	}
	requiredFields := []string{"name", "protocol", "base_url", "model"}
	for i, p := range cfg.Providers {
		var missing []string
		values := map[string]string{
			"name":     p.Name,
			"protocol": p.Protocol,
			"base_url": p.BaseURL,
			"model":    p.Model,
		}
		for _, f := range requiredFields {
			if values[f] == "" {
				missing = append(missing, f)
			}
		}
		if len(missing) > 0 {
			return &ConfigError{
				Message: fmt.Sprintf("Provider #%d: missing fields: %s", i+1, strings.Join(missing, ", ")),
			}
		}
		if !validProtocols[p.Protocol] {
			return &ConfigError{
				Message: fmt.Sprintf("Provider #%d: invalid protocol '%s', must be one of: anthropic, openai, openai-compat", i+1, p.Protocol),
			}
		}
	}
	return nil
}

func LoadConfig(path string) (*AppConfig, error) {
	if path != "" {
		cfg, err := loadSingleFile(path)
		if err != nil {
			return nil, err
		}
		if err := validateProviders(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Failed to get working directory: %s", err)}
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".swiftx", "config.yaml"),
		filepath.Join(wd, ".swiftx", "config.yaml"),
		filepath.Join(wd, ".swiftx", "config.local.yaml"),
	}

	var merged *AppConfig
	for _, path := range candidates {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		layer, err := loadSingleFile(path)
		if err != nil {
			return nil, err
		}
		if merged == nil {
			merged = layer
		} else {
			merged = mergeConfig(merged, layer)
		}
	}

	if merged == nil {
		return nil, &ConfigError{Message: "No config file found. Expected .swiftx/config.yaml in project or ~/.swiftx/config.yaml"}
	}

	if err := validateProviders(merged); err != nil {
		return nil, err
	}
	return merged, nil
}
