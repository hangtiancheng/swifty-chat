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

// realisticMCPTool simulates a realistic MCP tool schema size.
// Modelled after actual Grafana/Playwright MCP Server tools, each with
// 5-10 parameters including full description, enum, and default fields.
type realisticMCPTool struct {
	name   string
	desc   string
	params map[string]any
}

func (t *realisticMCPTool) Name() string           { return t.name }
func (t *realisticMCPTool) Description() string    { return t.desc }
func (t *realisticMCPTool) Category() ToolCategory { return CategoryCommand }
func (t *realisticMCPTool) ShouldDefer() bool      { return true }
func (t *realisticMCPTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.name,
		"description": t.desc,
		"input_schema": map[string]any{
			"type":       "object",
			"required":   []string{"query", "datasource"},
			"properties": t.params,
		},
	}
}
func (t *realisticMCPTool) Execute(_ context.Context, _ map[string]any) ToolResult {
	return ToolResult{Output: "ok"}
}

// makeRealisticTools generates n simulated MCP tools with rich parameter
// sets. Their schema size closely matches real Grafana/Playwright tools
// (approximately 800-1200 bytes of JSON per tool).
func makeRealisticTools(n int) []*realisticMCPTool {
	templates := []struct {
		namePrefix string
		desc       string
		params     map[string]any
	}{
		{
			namePrefix: "mcp__grafana__query_prometheus",
			desc:       "Execute a PromQL query against the specified Prometheus datasource and return time-series or instant results. Supports range queries with configurable step and time window.",
			params: map[string]any{
				"expr":        map[string]any{"type": "string", "description": "PromQL query expression to evaluate against the datasource"},
				"datasource":  map[string]any{"type": "string", "description": "Name or UID of the Prometheus datasource to query"},
				"start":       map[string]any{"type": "string", "description": "Start of the time range in RFC3339 format or relative (e.g. 'now-1h')"},
				"end":         map[string]any{"type": "string", "description": "End of the time range in RFC3339 format or relative (e.g. 'now')"},
				"step":        map[string]any{"type": "string", "description": "Query resolution step width in Prometheus duration format (e.g. '15s', '1m')"},
				"format":      map[string]any{"type": "string", "description": "Output format for results", "enum": []string{"table", "timeseries", "json"}},
				"max_results": map[string]any{"type": "integer", "description": "Maximum number of time series to return", "default": 100},
				"legend":      map[string]any{"type": "string", "description": "Legend format template for result series names"},
			},
		},
		{
			namePrefix: "mcp__grafana__search_dashboards",
			desc:       "Search for Grafana dashboards by title, tag, or folder. Returns matching dashboards with metadata including UID, title, URL, folder, and tags.",
			params: map[string]any{
				"query":   map[string]any{"type": "string", "description": "Search query string to match against dashboard titles"},
				"tag":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Filter dashboards by tags (AND logic)"},
				"folder":  map[string]any{"type": "string", "description": "Folder title or UID to restrict search scope"},
				"starred": map[string]any{"type": "boolean", "description": "If true, only return starred dashboards"},
				"limit":   map[string]any{"type": "integer", "description": "Maximum number of dashboards to return", "default": 50},
				"sort":    map[string]any{"type": "string", "description": "Sort order for results", "enum": []string{"alpha-asc", "alpha-desc", "created-asc", "created-desc"}},
			},
		},
		{
			namePrefix: "mcp__playwright__browser_click",
			desc:       "Click an element on the page identified by a CSS selector or accessible role. Supports options for button type, click count, position offset, force click, and timeout.",
			params: map[string]any{
				"selector":   map[string]any{"type": "string", "description": "CSS selector, XPath, or text selector to identify the target element"},
				"button":     map[string]any{"type": "string", "description": "Mouse button to click", "enum": []string{"left", "right", "middle"}, "default": "left"},
				"clickCount": map[string]any{"type": "integer", "description": "Number of clicks (1 for single, 2 for double)", "default": 1},
				"force":      map[string]any{"type": "boolean", "description": "Whether to bypass actionability checks and force the click"},
				"timeout":    map[string]any{"type": "integer", "description": "Maximum time in milliseconds to wait for the element", "default": 30000},
				"position":   map[string]any{"type": "object", "description": "Offset position relative to element's top-left corner", "properties": map[string]any{"x": map[string]any{"type": "number"}, "y": map[string]any{"type": "number"}}},
				"modifiers":  map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"Alt", "Control", "Meta", "Shift"}}, "description": "Keyboard modifiers to press during click"},
			},
		},
		{
			namePrefix: "mcp__grafana__query_loki",
			desc:       "Run a LogQL query against a Loki datasource and return matching log lines or metric results. Supports log queries, metric queries, and pattern-based aggregation.",
			params: map[string]any{
				"query":      map[string]any{"type": "string", "description": "LogQL query expression to execute"},
				"datasource": map[string]any{"type": "string", "description": "Name or UID of the Loki datasource"},
				"start":      map[string]any{"type": "string", "description": "Start timestamp in RFC3339 or relative format"},
				"end":        map[string]any{"type": "string", "description": "End timestamp in RFC3339 or relative format"},
				"limit":      map[string]any{"type": "integer", "description": "Maximum number of log entries to return", "default": 1000},
				"direction":  map[string]any{"type": "string", "description": "Log ordering direction", "enum": []string{"forward", "backward"}},
				"step":       map[string]any{"type": "string", "description": "Step interval for metric queries"},
				"dedup":      map[string]any{"type": "boolean", "description": "Whether to deduplicate log lines with same content"},
			},
		},
		{
			namePrefix: "mcp__playwright__browser_fill",
			desc:       "Fill an input field with text. Clears existing content before typing. Works with input, textarea, and contenteditable elements. Dispatches input and change events.",
			params: map[string]any{
				"selector":    map[string]any{"type": "string", "description": "CSS selector or text selector for the input element to fill"},
				"value":       map[string]any{"type": "string", "description": "Text value to fill into the input field"},
				"force":       map[string]any{"type": "boolean", "description": "Whether to bypass actionability checks"},
				"timeout":     map[string]any{"type": "integer", "description": "Maximum time in milliseconds to wait for element", "default": 30000},
				"noWaitAfter": map[string]any{"type": "boolean", "description": "If true, do not wait for navigation events after filling"},
			},
		},
		{
			namePrefix: "mcp__grafana__create_annotation",
			desc:       "Create an annotation on a Grafana dashboard panel or at the global level. Annotations mark important events on time-series graphs with optional tags and rich text descriptions.",
			params: map[string]any{
				"dashboardUID": map[string]any{"type": "string", "description": "UID of the dashboard to annotate (omit for global annotation)"},
				"panelId":      map[string]any{"type": "integer", "description": "Panel ID within the dashboard to annotate"},
				"time":         map[string]any{"type": "integer", "description": "Unix timestamp in milliseconds for annotation start"},
				"timeEnd":      map[string]any{"type": "integer", "description": "Unix timestamp in milliseconds for annotation end (for range annotations)"},
				"text":         map[string]any{"type": "string", "description": "Annotation description text, supports basic HTML formatting"},
				"tags":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags to associate with the annotation for filtering"},
			},
		},
	}

	tools := make([]*realisticMCPTool, n)
	for i := range n {
		tmpl := templates[i%len(templates)]
		tools[i] = &realisticMCPTool{
			name:   fmt.Sprintf("%s_%03d", tmpl.namePrefix, i),
			desc:   tmpl.desc,
			params: tmpl.params,
		}
	}
	return tools
}

// builtinToolSchemaSize returns the total schema JSON size of the built-in
// tools (ReadFile/WriteFile/EditFile/Bash/Glob/Grep).
func builtinToolSchemaSize() int {
	reg := CreateDefaultRegistry()
	schemas := reg.GetAllSchemas("anthropic")
	data, _ := json.Marshal(schemas)
	return len(data)
}

// TestDeferredBenchmarkFullSession simulates a complete multi-turn session,
// comparing the cumulative token cost in the tools parameter between an
// eager-loading strategy and a deferred-loading strategy.
//
// Scenario:
//   - 6 built-in tools (ReadFile/WriteFile/EditFile/Bash/Glob/Grep)
//   - 1 ToolSearch tool (deferred-loading only)
//   - 58 MCP tools (simulating 5 MCP servers)
//   - 10 conversation turns; one MCP tool is activated on turn 3 and another on turn 6
//
// Metric: the cumulative byte count of the tools JSON parameter across all
// turns. The deferred strategy also accounts for the overhead of the
// deferred tool name list injected via system-reminder.
func TestDeferredBenchmarkFullSession(t *testing.T) {
	const (
		numMCPTools = 58
		numTurns    = 10
	)

	// --- Build two registries ---

	// Eager loading: no tool is deferred.
	regFull := NewRegistry()
	for _, tool := range CreateDefaultTools().Registry.ListTools() {
		regFull.Register(tool)
	}
	mcpTools := makeRealisticTools(numMCPTools)
	// Eager strategy: MCP tools are registered as non-deferred (wrap to remove ShouldDefer).
	for _, mt := range mcpTools {
		regFull.Register(&nonDeferredWrapper{realisticMCPTool: mt})
	}

	// Deferred loading: all MCP tools are deferred.
	regDeferred := NewRegistry()
	for _, tool := range CreateDefaultTools().Registry.ListTools() {
		regDeferred.Register(tool)
	}
	toolSearch := &ToolSearchTool{Registry: regDeferred, Protocol: "anthropic"}
	regDeferred.Register(toolSearch)
	for _, mt := range mcpTools {
		regDeferred.Register(mt)
	}

	// --- Simulate session ---

	var totalBytesFull int
	var totalBytesDeferred int

	// Activate one tool on turn 3 and another on turn 6.
	activateAtTurn := map[int]string{
		3: mcpTools[5].Name(),
		6: mcpTools[20].Name(),
	}

	t.Logf("=== Session simulation: %d built-in tools + %d MCP tools, %d turns ===\n", 6, numMCPTools, numTurns)
	t.Logf("%-6s %12s %12s %12s   %s", "Turn", "Full(bytes)", "Deferred(bytes)", "Deferred-detail", "Event")

	for turn := 1; turn <= numTurns; turn++ {
		event := ""

		// Activate a tool (simulates ToolSearch being called).
		if toolName, ok := activateAtTurn[turn]; ok {
			regDeferred.MarkDiscovered(toolName)
			event = fmt.Sprintf("activated %s", toolName)
		}

		// Eager strategy: tools parameter size for this turn.
		schemasFull := regFull.GetAllSchemas("anthropic")
		dataFull, _ := json.Marshal(schemasFull)
		turnBytesFull := len(dataFull)

		// Deferred strategy: tools parameter + deferred tool name list in system-reminder.
		schemasDeferred := regDeferred.GetAllSchemas("anthropic")
		dataDeferred, _ := json.Marshal(schemasDeferred)
		turnToolsBytes := len(dataDeferred)

		// Deferred tool name list injected via system-reminder each turn.
		deferredNames := regDeferred.GetDeferredToolNames()
		reminderText := ""
		if len(deferredNames) > 0 {
			reminderText = "The following deferred tools are available via ToolSearch. Their schemas are NOT loaded - use ToolSearch with query \"select:<name>[,<name>...]\" to load tool schemas before calling them:\n" + strings.Join(deferredNames, "\n")
		}
		turnReminderBytes := len(reminderText)
		turnBytesDeferred := turnToolsBytes + turnReminderBytes

		totalBytesFull += turnBytesFull
		totalBytesDeferred += turnBytesDeferred

		t.Logf("%-6d %12d %12d   (tools=%d + reminder=%d)   %s",
			turn, turnBytesFull, turnBytesDeferred, turnToolsBytes, turnReminderBytes, event)
	}

	savings := 1 - float64(totalBytesDeferred)/float64(totalBytesFull)
	estimatedTokensFull := totalBytesFull / 4
	estimatedTokensDeferred := totalBytesDeferred / 4
	tokensSaved := estimatedTokensFull - estimatedTokensDeferred

	t.Logf("")
	t.Logf("=== Full session summary ===")
	t.Logf("Eager loading total:   %d bytes (%d estimated tokens)", totalBytesFull, estimatedTokensFull)
	t.Logf("Deferred loading total:   %d bytes (%d estimated tokens)", totalBytesDeferred, estimatedTokensDeferred)
	t.Logf("Savings:           %.1f%% (%d estimated tokens saved)", savings*100, tokensSaved)
	t.Logf("")

	// Per-turn snapshot comparison.
	schemasFull1 := regFull.GetAllSchemas("anthropic")
	dataFull1, _ := json.Marshal(schemasFull1)
	t.Logf("=== Single-turn snapshot (final state, 2 MCP tools activated) ===")
	t.Logf("Eager tools parameter:   %d bytes (%d tools)", len(dataFull1), len(schemasFull1))

	schemasD1 := regDeferred.GetAllSchemas("anthropic")
	dataD1, _ := json.Marshal(schemasD1)
	dNames := regDeferred.GetDeferredToolNames()
	reminder := strings.Join(dNames, "\n")
	t.Logf("Deferred tools parameter:   %d bytes (%d tools) + reminder %d bytes (%d names)",
		len(dataD1), len(schemasD1), len(reminder), len(dNames))

	if savings < 0.80 {
		t.Errorf("full-session token savings should be >= 80%%, got %.1f%%", savings*100)
	}
}

// nonDeferredWrapper wraps a realisticMCPTool as a non-deferred version
// for comparison with the eager-loading strategy.
type nonDeferredWrapper struct {
	*realisticMCPTool
}

func (w *nonDeferredWrapper) ShouldDefer() bool { return false }
