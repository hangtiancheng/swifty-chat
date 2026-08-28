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

package mcp

import (
	"context"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

type stubMCPTool struct {
	name    string
	noDefer bool
}

func (s *stubMCPTool) Name() string                   { return s.name }
func (s *stubMCPTool) Description() string            { return "stub" }
func (s *stubMCPTool) Category() tools.ToolCategory   { return tools.CategoryCommand }
func (s *stubMCPTool) ShouldDefer() bool              { return !s.noDefer }
func (s *stubMCPTool) SetDeferLoading(on bool)        { s.noDefer = !on }
func (s *stubMCPTool) MCPServerName() string          { return "linear" }
func (s *stubMCPTool) MCPInputSchema() map[string]any { return map[string]any{"type": "object"} }

func (s *stubMCPTool) Schema() map[string]any {
	return map[string]any{
		"name":         s.name,
		"description":  "stub",
		"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func (s *stubMCPTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	return tools.ToolResult{Output: "ok"}
}

func TestIsOfficialAnthropicEndpoint(t *testing.T) {
	// An empty base_url means the SDK default address, which is official
	if !IsOfficialAnthropicEndpoint("") {
		t.Error("empty base_url should be treated as official")
	}
	if !IsOfficialAnthropicEndpoint("https://api.anthropic.com") {
		t.Error("official domain should be treated as official")
	}
	if IsOfficialAnthropicEndpoint("https://api.minimaxi.com/anthropic") {
		t.Error("third-party endpoint should not be treated as official")
	}
}

func TestDecideMode(t *testing.T) {
	const window = 200000 // 10% is 20k tokens, roughly 50k characters
	cases := []struct {
		desc    string
		baseURL string
		chars   int
		want    tools.McpLoadingMode
	}{
		{"small schema loads eagerly", "https://proxy.example.com", 1000, tools.McpLoadingEager},
		{"no MCP tools also loads eagerly", "https://proxy.example.com", 0, tools.McpLoadingEager},
		{"official endpoint uses native deferral", "", 500000, tools.McpLoadingNative},
		{"third-party endpoint uses McpCall", "https://api.minimaxi.com/anthropic", 500000, tools.McpLoadingDispatch},
	}
	for _, c := range cases {
		if got := DecideMode(c.baseURL, window, c.chars, DefaultEagerThresholdPercent); got != c.want {
			t.Errorf("%s: got %s, want %s", c.desc, got, c.want)
		}
	}
}

func TestDecideModeEnvOverride(t *testing.T) {
	t.Setenv(envLoadingOverride, "dispatch")
	// Small config should be eager but is forced to dispatch by env var
	if got := DecideMode("", 200000, 10, DefaultEagerThresholdPercent); got != tools.McpLoadingDispatch {
		t.Errorf("env var should override the decision, got %s", got)
	}
}

func TestMeasureSchemaCharsCountsOnlyMCPTools(t *testing.T) {
	reg := tools.CreateDefaultRegistry()
	if got := MeasureSchemaChars(reg); got != 0 {
		t.Errorf("built-in tools only should yield 0, got %d", got)
	}
	reg.Register(&stubMCPTool{name: "mcp__linear__create_issue"})
	if got := MeasureSchemaChars(reg); got <= 0 {
		t.Errorf("with MCP tools should be > 0, got %d", got)
	}
}

func TestApplyMode(t *testing.T) {
	mcpCount := func(reg *tools.Registry) (total, deferred int) {
		for _, s := range reg.GetAllSchemas("anthropic") {
			name, _ := s["name"].(string)
			if len(name) > 5 && name[:5] == "mcp__" {
				total++
				if s["defer_loading"] == true {
					deferred++
				}
			}
		}
		return
	}

	// eager: enters tools[] without defer_loading
	reg := tools.CreateDefaultRegistry()
	reg.Register(&stubMCPTool{name: "mcp__linear__create_issue"})
	ApplyMode(reg, tools.McpLoadingEager)
	if total, deferred := mcpCount(reg); total != 1 || deferred != 0 {
		t.Errorf("eager: expected in array without flag, got total=%d deferred=%d", total, deferred)
	}

	// native: enters tools[] with defer_loading; the server decides visibility
	reg = tools.CreateDefaultRegistry()
	reg.Register(&stubMCPTool{name: "mcp__linear__create_issue"})
	ApplyMode(reg, tools.McpLoadingNative)
	if total, deferred := mcpCount(reg); total != 1 || deferred != 1 {
		t.Errorf("native: expected in array with flag, got total=%d deferred=%d", total, deferred)
	}

	// dispatch: never enters tools[]; relies on McpCall
	reg = tools.CreateDefaultRegistry()
	reg.Register(&stubMCPTool{name: "mcp__linear__create_issue"})
	ApplyMode(reg, tools.McpLoadingDispatch)
	if total, _ := mcpCount(reg); total != 0 {
		t.Errorf("dispatch: MCP tools should not appear in array, got %d", total)
	}
}

func TestNativeFlagNotSentOnOpenAIProtocol(t *testing.T) {
	// defer_loading is an Anthropic field; it must not leak under the openai protocol
	reg := tools.CreateDefaultRegistry()
	reg.Register(&stubMCPTool{name: "mcp__linear__create_issue"})
	ApplyMode(reg, tools.McpLoadingNative)
	for _, s := range reg.GetAllSchemas("openai") {
		if name, _ := s["name"].(string); len(name) > 5 && name[:5] == "mcp__" {
			t.Errorf("MCP tools should not leak under openai protocol: %s", name)
		}
	}
}

func TestMCPToolNamePrefixSanitizes(t *testing.T) {
	// Hyphens in server names must be replaced with underscores, otherwise prefix-based filtering will miss tools
	if got := MCPToolNamePrefix("chrome-devtools"); got != "mcp__chrome_devtools__" {
		t.Errorf("got %q", got)
	}
}
