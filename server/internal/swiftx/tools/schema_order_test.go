package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type orderPlainTool struct{ n string }

func (t *orderPlainTool) Name() string           { return t.n }
func (t *orderPlainTool) Description() string    { return t.n }
func (t *orderPlainTool) Category() ToolCategory { return CategoryRead }
func (t *orderPlainTool) Schema() map[string]any {
	return map[string]any{"name": t.n, "input_schema": map[string]any{"type": "object"}}
}
func (t *orderPlainTool) Execute(context.Context, map[string]any) ToolResult {
	return ToolResult{Output: "ok"}
}

type orderDeferTool struct{ n string }

func (t *orderDeferTool) Name() string           { return t.n }
func (t *orderDeferTool) Description() string    { return t.n }
func (t *orderDeferTool) Category() ToolCategory { return CategoryRead }
func (t *orderDeferTool) ShouldDefer() bool      { return true }
func (t *orderDeferTool) Schema() map[string]any {
	return map[string]any{"name": t.n, "input_schema": map[string]any{"type": "object"}}
}
func (t *orderDeferTool) Execute(context.Context, map[string]any) ToolResult {
	return ToolResult{Output: "ok"}
}

func schemaNames(schemas []map[string]any) string {
	var out []string
	for _, s := range schemas {
		out = append(out, s["name"].(string))
	}
	return strings.Join(out, ",")
}

// The tool list order must be identical across calls.
//
// tools is a map and Go map iteration order is random. The tool list is rendered after the system
// prompt and before messages; any order change alters the bytes of the entire block, invalidating
// the conversation history cache that follows — the cost is the same as actually adding a tool,
// even though the content is unchanged.
func TestGetAllSchemasOrderIsStable(t *testing.T) {
	reg := NewRegistry()
	for i := 0; i < 20; i++ {
		reg.Register(&orderPlainTool{n: fmt.Sprintf("Tool%02d", i)})
	}
	for i := 0; i < 10; i++ {
		reg.Register(&orderDeferTool{n: fmt.Sprintf("mcp__srv__t%02d", i)})
	}
	reg.McpLoadingMode = McpLoadingNative
	reg.ExposeToolSearch = true

	want := schemaNames(reg.GetAllSchemas("anthropic"))
	for i := 0; i < 100; i++ {
		if got := schemaNames(reg.GetAllSchemas("anthropic")); got != want {
			t.Fatalf("order changed on iteration %d\nfirst: %s\nthis:  %s", i, want, got)
		}
	}

	// The openai protocol goes through the same sorting path.
	wantOpenAI := schemaNames(reg.GetAllSchemas("openai"))
	for i := 0; i < 20; i++ {
		if got := schemaNames(reg.GetAllSchemas("openai")); got != wantOpenAI {
			t.Fatalf("openai protocol order changed on iteration %d", i)
		}
	}
}

// The cache breakpoint must not land on a tool with defer_loading: the official endpoint rejects
// a request when a tool carries both defer_loading and cache_control.
//
// This test asserts that after sorting, tools[] always contains a non-deferred tool that can
// serve as the landing spot, and that the tail may indeed be a deferred tool (i.e. naively marking
// the last element would fail). The actual marking logic lives in internal/llm/anthropic.go.
func TestNonDeferredToolExistsForCacheMarker(t *testing.T) {
	reg := NewRegistry()
	for _, n := range []string{"ReadFile", "WriteFile", "Bash", "ToolSearch"} {
		reg.Register(&orderPlainTool{n: n})
	}
	for i := 0; i < 5; i++ {
		reg.Register(&orderDeferTool{n: fmt.Sprintf("mcp__srv__z%02d", i)})
	}
	reg.McpLoadingMode = McpLoadingNative
	reg.ExposeToolSearch = true

	schemas := reg.GetAllSchemas("anthropic")
	lastDeferred, _ := schemas[len(schemas)-1]["defer_loading"].(bool)
	if !lastDeferred {
		t.Fatal("test precondition broken: expected the last sorted tool to be deferred")
	}

	// Scanning backward from the tail must find a non-deferred landing spot; this is the
	// precondition relied upon by anthropic.go.
	found := -1
	for i := len(schemas) - 1; i >= 0; i-- {
		if dl, _ := schemas[i]["defer_loading"].(bool); !dl {
			found = i
			break
		}
	}
	if found < 0 {
		t.Fatal("no non-deferred tool found; nowhere to place the cache breakpoint")
	}
	if name := schemas[found]["name"].(string); strings.HasPrefix(name, MCPToolPrefix) {
		t.Errorf("landing spot %s is an MCP tool and should not be selected", name)
	}
}
