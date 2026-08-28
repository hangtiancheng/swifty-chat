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

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// mockDeferredTool simulates a tool that will be deferred.
type mockDeferredTool struct {
	name string
	desc string
}

func (t *mockDeferredTool) Name() string           { return t.name }
func (t *mockDeferredTool) Description() string    { return t.desc }
func (t *mockDeferredTool) Category() ToolCategory { return CategoryCommand }
func (t *mockDeferredTool) ShouldDefer() bool      { return true }

func (t *mockDeferredTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.name,
		"description": t.desc,
		"input_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"arg1": map[string]any{"type": "string"}},
		},
	}
}
func (t *mockDeferredTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	return ToolResult{Output: "ok"}
}

// mockNormalTool simulates a normal tool that is never deferred.
type mockNormalTool struct {
	name string
}

func (t *mockNormalTool) Name() string           { return t.name }
func (t *mockNormalTool) Description() string    { return "normal tool" }
func (t *mockNormalTool) Category() ToolCategory { return CategoryRead }
func (t *mockNormalTool) Schema() map[string]any {
	return map[string]any{
		"name":         t.name,
		"description":  "normal tool",
		"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
	}
}
func (t *mockNormalTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	return ToolResult{Output: "ok"}
}

// mockMCPTool simulates MCPToolWrapper (does not implement DeferrableTool).
type mockMCPTool struct {
	name string
	desc string
}

func (t *mockMCPTool) Name() string           { return t.name }
func (t *mockMCPTool) Description() string    { return t.desc }
func (t *mockMCPTool) Category() ToolCategory { return CategoryCommand }
func (t *mockMCPTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.name,
		"description": t.desc,
		"input_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"expr": map[string]any{"type": "string"}},
		},
	}
}
func (t *mockMCPTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	return ToolResult{Output: "ok"}
}

func TestDeferredToolsNotInGetAllSchemas(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockNormalTool{name: "ReadFile"})
	reg.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus"})
	reg.Register(&mockDeferredTool{name: "mcp__grafana__search", desc: "Search dashboards"})

	schemas := reg.GetAllSchemas("anthropic")

	// Only ReadFile should appear; deferred tools should be excluded.
	if len(schemas) != 1 {
		t.Errorf("expected 1 schema (only ReadFile), got %d", len(schemas))
	}
	if schemas[0]["name"] != "ReadFile" {
		t.Errorf("expected ReadFile, got %s", schemas[0]["name"])
	}
}

func TestMCPToolIsDeferred(t *testing.T) {
	// mockMCPTool does not implement DeferrableTool, simulating legacy behaviour (no defer).
	// The real MCPToolWrapper now implements ShouldDefer() = true.
	reg := NewRegistry()
	reg.Register(&mockNormalTool{name: "ReadFile"})
	reg.Register(&mockMCPTool{name: "mcp__grafana__query", desc: "Query Prometheus"})

	schemas := reg.GetAllSchemas("anthropic")
	// mockMCPTool does not implement DeferrableTool, so it is still included eagerly.
	if len(schemas) != 2 {
		t.Errorf("expected 2 schemas (mockMCPTool has no ShouldDefer), got %d", len(schemas))
	}

	// But when using mockDeferredTool to simulate the real MCPToolWrapper behaviour:
	reg2 := NewRegistry()
	reg2.Register(&mockNormalTool{name: "ReadFile"})
	reg2.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus"})

	schemas2 := reg2.GetAllSchemas("anthropic")
	if len(schemas2) != 1 {
		t.Errorf("expected 1 schema (deferred MCP tool excluded), got %d", len(schemas2))
	}

	deferred := reg2.GetDeferredTools()
	if len(deferred) != 1 {
		t.Errorf("expected 1 deferred tool, got %d", len(deferred))
	}
}

func TestDiscoveredToolsIncludedInSchemas(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockNormalTool{name: "ReadFile"})
	reg.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus"})
	reg.Register(&mockDeferredTool{name: "mcp__grafana__search", desc: "Search dashboards"})

	// Before discovery: only ReadFile
	schemas := reg.GetAllSchemas("anthropic")
	if len(schemas) != 1 {
		t.Errorf("before discovery: expected 1 schema, got %d", len(schemas))
	}

	// Discover one tool
	reg.MarkDiscovered("mcp__grafana__query")

	// After discovery: ReadFile + discovered tool
	schemas = reg.GetAllSchemas("anthropic")
	if len(schemas) != 2 {
		t.Errorf("after discovery: expected 2 schemas, got %d", len(schemas))
	}

	// GetDeferredToolNames should only return undiscovered ones
	names := reg.GetDeferredToolNames()
	if len(names) != 1 || names[0] != "mcp__grafana__search" {
		t.Errorf("expected only mcp__grafana__search as deferred, got %v", names)
	}
}

// MCP tools are never marked as discovered: they never enter tools[]; calls
// go through McpCall. This keeps the tools array byte-identical for the entire
// session, so the prompt cache prefix is never invalidated.
func TestToolSearchDoesNotMarkMCPToolsDiscovered(t *testing.T) {
	reg := NewRegistry()
	reg.McpLoadingMode = McpLoadingDispatch
	reg.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus"})

	ts := &ToolSearchTool{Registry: reg, Protocol: "anthropic"}
	res := ts.Execute(context.Background(), map[string]any{"query": "select:mcp__grafana__query"})

	if reg.IsDiscovered("mcp__grafana__query") {
		t.Error("MCP tools should not be marked as discovered")
	}
	if len(reg.GetAllSchemas("anthropic")) != 0 {
		t.Error("MCP tools should not appear in tools[]")
	}
	// The raw schema must be visible to the model so it knows how to fill arguments
	if !strings.Contains(res.Output, "mcp__grafana__query") {
		t.Error("output should contain the tool schema")
	}
	// It must also indicate which entry point to use
	if !strings.Contains(res.Output, "McpCall") {
		t.Error("output should mention McpCall as the invocation path")
	}
}

// Non-MCP deferred tools have no McpCall entry point; they still use the
// legacy path of entering tools[].
func TestToolSearchStillMarksNonMCPDeferredTools(t *testing.T) {
	reg := NewRegistry()
	reg.McpLoadingMode = McpLoadingDispatch
	reg.Register(&mockDeferredTool{name: "SomeDeferredTool", desc: "not an MCP tool"})

	ts := &ToolSearchTool{Registry: reg, Protocol: "anthropic"}
	ts.Execute(context.Background(), map[string]any{"query": "select:SomeDeferredTool"})

	if !reg.IsDiscovered("SomeDeferredTool") {
		t.Error("non-MCP deferred tools should still be marked as discovered")
	}
	if len(reg.GetAllSchemas("anthropic")) != 1 {
		t.Error("marked tools should appear in tools[]")
	}
}

// On the official endpoint, return tool_reference; the server expands the
// schema and the tools array stays unchanged.
func TestToolSearchReturnsToolReferenceInNativeMode(t *testing.T) {
	reg := NewRegistry()
	reg.McpLoadingMode = McpLoadingNative
	reg.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus"})

	ts := &ToolSearchTool{Registry: reg, Protocol: "anthropic"}
	res := ts.Execute(context.Background(), map[string]any{"query": "select:mcp__grafana__query"})

	if len(res.ContentBlocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(res.ContentBlocks))
	}
	if res.ContentBlocks[0]["type"] != "tool_reference" {
		t.Errorf("block type should be tool_reference, got %v", res.ContentBlocks[0]["type"])
	}
	if res.ContentBlocks[0]["tool_name"] != "mcp__grafana__query" {
		t.Errorf("unexpected tool_name: %v", res.ContentBlocks[0]["tool_name"])
	}
	// In native mode tools stay in the array with defer_loading; the server decides visibility
	schemas := reg.GetAllSchemas("anthropic")
	if len(schemas) != 1 || schemas[0]["defer_loading"] != true {
		t.Errorf("native mode should keep tools in tools[] with defer_loading, got %v", schemas)
	}
}
func TestToolSearchSelect(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus"})
	reg.Register(&mockDeferredTool{name: "mcp__grafana__search", desc: "Search dashboards"})

	ts := &ToolSearchTool{Registry: reg, Protocol: "anthropic"}
	result := ts.Execute(context.Background(), map[string]any{
		"query": "select:mcp__grafana__query",
	})

	if result.IsError {
		t.Errorf("ToolSearch returned error: %s", result.Output)
	}
	if !contains(result.Output, "mcp__grafana__query") {
		t.Errorf("expected output to contain tool name, got: %s", result.Output)
	}
}

func TestToolSearchKeyword(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus metrics"})
	reg.Register(&mockDeferredTool{name: "mcp__github__issues", desc: "List GitHub issues"})

	ts := &ToolSearchTool{Registry: reg, Protocol: "anthropic"}
	result := ts.Execute(context.Background(), map[string]any{
		"query": "prometheus",
	})

	if result.IsError {
		t.Errorf("ToolSearch returned error: %s", result.Output)
	}
	if !contains(result.Output, "mcp__grafana__query") {
		t.Errorf("expected to find grafana query tool, got: %s", result.Output)
	}
}

func TestToolSearchNoMatch(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockDeferredTool{name: "mcp__grafana__query", desc: "Query Prometheus"})

	ts := &ToolSearchTool{Registry: reg, Protocol: "anthropic"}
	result := ts.Execute(context.Background(), map[string]any{
		"query": "nonexistent_xyz",
	})

	if result.IsError {
		t.Errorf("should not be error, got: %s", result.Output)
	}
	if !contains(result.Output, "No matching") {
		t.Errorf("expected 'No matching' message, got: %s", result.Output)
	}
}

// mockLargeDeferredTool simulates a realistic MCP tool with a large schema
// (~500+ chars of JSON per tool, mimicking real Grafana/Playwright tools).
type mockLargeDeferredTool struct {
	name string
	desc string
}

func (t *mockLargeDeferredTool) Name() string           { return t.name }
func (t *mockLargeDeferredTool) Description() string    { return t.desc }
func (t *mockLargeDeferredTool) Category() ToolCategory { return CategoryCommand }
func (t *mockLargeDeferredTool) ShouldDefer() bool      { return true }
func (t *mockLargeDeferredTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.name,
		"description": t.desc,
		"input_schema": map[string]any{
			"type":     "object",
			"required": []string{"query", "datasource"},
			"properties": map[string]any{
				"query":       map[string]any{"type": "string", "description": "The query expression to execute against the datasource"},
				"datasource":  map[string]any{"type": "string", "description": "Name or UID of the target datasource to query"},
				"start_time":  map[string]any{"type": "string", "description": "Start of the time range in RFC3339 or relative format"},
				"end_time":    map[string]any{"type": "string", "description": "End of the time range in RFC3339 or relative format"},
				"step":        map[string]any{"type": "string", "description": "Query resolution step width in duration format"},
				"max_results": map[string]any{"type": "integer", "description": "Maximum number of results to return from the query"},
				"format":      map[string]any{"type": "string", "description": "Output format: table, timeseries, or json"},
				"labels":      map[string]any{"type": "object", "description": "Additional label matchers to filter results"},
			},
		},
	}
}

func (t *mockLargeDeferredTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	return ToolResult{Output: "ok"}
}

func TestDeferredTokenSavings(t *testing.T) {
	reg := NewRegistry()

	// Register 2 normal (non-deferred) tools with small schemas.
	reg.Register(&mockNormalTool{name: "ReadFile"})
	reg.Register(&mockNormalTool{name: "WriteFile"})

	// Register 50 deferred tools with realistic large schemas.
	for i := range 50 {
		reg.Register(&mockLargeDeferredTool{
			name: fmt.Sprintf("mcp__grafana__tool_%03d", i),
			desc: fmt.Sprintf("A realistic MCP tool that queries datasource %d with full parameter set", i),
		})
	}

	// Measure size with deferred tools hidden (default state).
	schemasDeferred := reg.GetAllSchemas("anthropic")
	bytesDeferred, err := json.Marshal(schemasDeferred)
	if err != nil {
		t.Fatalf("json.Marshal deferred schemas: %v", err)
	}
	sizeDeferred := len(bytesDeferred)

	// Discover all 50 deferred tools.
	for i := range 50 {
		reg.MarkDiscovered(fmt.Sprintf("mcp__grafana__tool_%03d", i))
	}

	// Measure size with all tools included.
	schemasAll := reg.GetAllSchemas("anthropic")
	bytesAll, err := json.Marshal(schemasAll)
	if err != nil {
		t.Fatalf("json.Marshal all schemas: %v", err)
	}
	sizeAll := len(bytesAll)

	savings := 1 - float64(sizeDeferred)/float64(sizeAll)
	t.Logf("Deferred size: %d bytes, Full size: %d bytes, Savings: %.2f%%", sizeDeferred, sizeAll, savings*100)

	if savings < 0.90 {
		t.Errorf("expected >= 90%% token savings from deferral, got %.2f%%", savings*100)
	}
}

func TestDeferredEndToEndDiscovery(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockNormalTool{name: "Bash"})
	reg.Register(&mockDeferredTool{name: "mcp__playwright__click", desc: "Click an element"})
	reg.Register(&mockDeferredTool{name: "mcp__playwright__fill", desc: "Fill a form field"})

	// Step 1: Deferred tools should NOT appear in GetAllSchemas.
	schemas := reg.GetAllSchemas("anthropic")
	for _, s := range schemas {
		name := s["name"].(string)
		if name == "mcp__playwright__click" || name == "mcp__playwright__fill" {
			t.Errorf("deferred tool %q should not appear in GetAllSchemas before discovery", name)
		}
	}
	if len(schemas) != 1 {
		t.Errorf("expected 1 schema (Bash only), got %d", len(schemas))
	}

	// Step 2: Both deferred tool names should be returned by GetDeferredToolNames.
	deferredNames := reg.GetDeferredToolNames()
	nameSet := make(map[string]bool)
	for _, n := range deferredNames {
		nameSet[n] = true
	}
	if !nameSet["mcp__playwright__click"] || !nameSet["mcp__playwright__fill"] {
		t.Errorf("expected both deferred tools in GetDeferredToolNames, got %v", deferredNames)
	}

	// Step 3: Discover one tool.
	reg.MarkDiscovered("mcp__playwright__click")

	// Step 4: The discovered tool should now appear in GetAllSchemas.
	schemas = reg.GetAllSchemas("anthropic")
	foundClick := false
	foundFill := false
	for _, s := range schemas {
		switch s["name"].(string) {
		case "mcp__playwright__click":
			foundClick = true
		case "mcp__playwright__fill":
			foundFill = true
		}
	}
	if !foundClick {
		t.Error("mcp__playwright__click should appear in GetAllSchemas after MarkDiscovered")
	}
	if foundFill {
		t.Error("mcp__playwright__fill should NOT appear in GetAllSchemas (still deferred)")
	}
	if len(schemas) != 2 {
		t.Errorf("expected 2 schemas (Bash + click), got %d", len(schemas))
	}

	// Step 5: GetDeferredToolNames should only return the undiscovered tool.
	deferredNames = reg.GetDeferredToolNames()
	if len(deferredNames) != 1 {
		t.Errorf("expected 1 deferred tool remaining, got %d: %v", len(deferredNames), deferredNames)
	}
	if len(deferredNames) == 1 && deferredNames[0] != "mcp__playwright__fill" {
		t.Errorf("expected mcp__playwright__fill as only deferred tool, got %q", deferredNames[0])
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
