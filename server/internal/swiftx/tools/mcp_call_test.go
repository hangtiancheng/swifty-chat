package tools

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// mockDispatchTool is a minimal MCP tool stand-in: it exposes a schema and
// records the arguments it receives.
type mockDispatchTool struct {
	name     string
	server   string
	schema   map[string]any
	received map[string]any
	noDefer  bool
}

func (m *mockDispatchTool) Name() string            { return m.name }
func (m *mockDispatchTool) Description() string     { return "mock" }
func (m *mockDispatchTool) Category() ToolCategory  { return CategoryCommand }
func (m *mockDispatchTool) ShouldDefer() bool       { return !m.noDefer }
func (m *mockDispatchTool) SetDeferLoading(on bool) { m.noDefer = !on }
func (m *mockDispatchTool) MCPServerName() string   { return m.server }
func (m *mockDispatchTool) MCPInputSchema() map[string]any {
	if m.schema == nil {
		return map[string]any{}
	}
	return m.schema
}

func (m *mockDispatchTool) Schema() map[string]any {
	return map[string]any{
		"name":         m.name,
		"description":  "mock",
		"input_schema": m.MCPInputSchema(),
	}
}

func (m *mockDispatchTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	m.received = args
	return ToolResult{Output: "ok"}
}

var testSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"issueId": map[string]any{"type": "string"},
		"limit":   map[string]any{"type": "integer"},
		"ratio":   map[string]any{"type": "number"},
		"flag":    map[string]any{"type": "boolean"},
		"labels":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"ports":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"config": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"replicas": map[string]any{"type": "integer"},
				"features": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
	},
}

// Coercion contract: these seven cases must be identical across all four languages.
func TestCoerceBySchemaContract(t *testing.T) {
	cases := []struct {
		desc  string
		given map[string]any
		want  map[string]any
	}{
		{"string from integer", map[string]any{"issueId": float64(8891)}, map[string]any{"issueId": "8891"}},
		{"string from float", map[string]any{"issueId": 1.5}, map[string]any{"issueId": "1.5"}},
		{"integer from numeric string", map[string]any{"limit": "5"}, map[string]any{"limit": int64(5)}},
		{"number from padded numeric string", map[string]any{"ratio": " 1.5 "}, map[string]any{"ratio": 1.5}},
		{"boolean ← true", map[string]any{"flag": "true"}, map[string]any{"flag": true}},
		{"boolean from uppercase FALSE", map[string]any{"flag": "FALSE"}, map[string]any{"flag": false}},
		{
			"array from single-key object unwrap",
			map[string]any{"labels": map[string]any{"item": []any{"a", "b"}}},
			map[string]any{"labels": []any{"a", "b"}},
		},
		{
			"array from comma-separated string",
			map[string]any{"labels": "a, b"},
			map[string]any{"labels": []any{"a", "b"}},
		},
		{
			"array recurses per items schema",
			map[string]any{"ports": []any{"8080", "9090"}},
			map[string]any{"ports": []any{int64(8080), int64(9090)}},
		},
		{
			"object recurses per properties, including nested levels",
			map[string]any{"config": map[string]any{
				"replicas": "4",
				"features": map[string]any{"item": []any{"x"}},
			}},
			map[string]any{"config": map[string]any{
				"replicas": int64(4),
				"features": []any{"x"},
			}},
		},
	}
	for _, c := range cases {
		got := CoerceBySchema(c.given, testSchema)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v, want %#v", c.desc, got, c.want)
		}
	}
}

func TestCoerceBySchemaLeavesThingsAlone(t *testing.T) {
	// bool must not be coerced to string via numeric path
	if got := CoerceBySchema(map[string]any{"issueId": true}, testSchema); got.(map[string]any)["issueId"] != true {
		t.Errorf("bool should not be coerced to string, got %#v", got)
	}
	// Uncoercible values pass through unchanged; the MCP server reports its own error
	if got := CoerceBySchema(map[string]any{"limit": "many"}, testSchema); got.(map[string]any)["limit"] != "many" {
		t.Error("uncoercible value should pass through unchanged")
	}
	// Keys absent from the schema are left untouched
	if got := CoerceBySchema(map[string]any{"extra": 1}, testSchema); got.(map[string]any)["extra"] != 1 {
		t.Error("unknown keys should pass through unchanged")
	}
	// Empty schema is a no-op
	if got := CoerceBySchema(map[string]any{"a": "1"}, map[string]any{}); got.(map[string]any)["a"] != "1" {
		t.Error("empty schema should not modify arguments")
	}
}

// Numeric parsing leniency varies by language: Go's ParseFloat accepts inf and
// scientific notation, Python's int() accepts underscores. These shapes must
// yield the same result across all four languages.
func TestCoerceNumericShapeParity(t *testing.T) {
	cases := []struct {
		key  string
		in   string
		want any
	}{
		{"limit", "5", int64(5)},
		{"limit", "+5", int64(5)},
		{"limit", "5.7", "5.7"}, // integer does not truncate
		{"limit", "1_000", "1_000"},
		{"limit", "1e3", "1e3"},
		{"limit", "5abc", "5abc"},
		{"ratio", " 1.5 ", 1.5},
		{"ratio", "1e3", 1000.0}, // scientific notation is valid JSON number, accept
		{"ratio", "inf", "inf"},
		{"ratio", "nan", "nan"},
	}
	for _, c := range cases {
		got := CoerceBySchema(map[string]any{c.key: c.in}, testSchema).(map[string]any)[c.key]
		if got != c.want {
			t.Errorf("%s=%q coerced to %#v, want %#v", c.key, c.in, got, c.want)
		}
	}
}

// Unwrapping only recognizes single-key objects; multi-key objects have
// ambiguous intent and pass through for the server to reject
func TestCoerceMultiKeyObjectForArrayLeftAlone(t *testing.T) {
	inner := map[string]any{"item": "metrics", "tracing": ""}
	got := CoerceBySchema(map[string]any{"labels": inner}, testSchema).(map[string]any)["labels"]
	m, ok := got.(map[string]any)
	if !ok || len(m) != 2 || m["item"] != "metrics" {
		t.Errorf("multi-key object should pass through unchanged, got %#v", got)
	}
}

func newDispatchRegistry() (*Registry, *McpCallTool, *mockDispatchTool) {
	reg := NewRegistry()
	reg.McpLoadingMode = McpLoadingDispatch
	tool := &mockDispatchTool{name: "mcp__linear__create_issue", server: "linear", schema: testSchema}
	reg.Register(tool)
	d := &McpCallTool{Registry: reg}
	reg.Register(d)
	return reg, d, tool
}

func TestMcpCallResolvesFullName(t *testing.T) {
	_, d, tool := newDispatchRegistry()
	res := d.Execute(context.Background(), map[string]any{
		"server": "linear", "tool": "mcp__linear__create_issue",
		"arguments": map[string]any{"issueId": "A"},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if tool.received["issueId"] != "A" {
		t.Errorf("arguments not delivered: %#v", tool.received)
	}
}

// Models frequently pass only the short name (~30% of calls in practice);
// this must be tolerated, otherwise it costs a needless retry round.
func TestMcpCallResolvesShortName(t *testing.T) {
	_, d, tool := newDispatchRegistry()
	res := d.Execute(context.Background(), map[string]any{
		"server": "linear", "tool": "create_issue",
		"arguments": map[string]any{"issueId": "A"},
	})
	if res.IsError {
		t.Fatalf("short name should resolve: %s", res.Output)
	}
	if tool.received["issueId"] != "A" {
		t.Errorf("arguments not delivered: %#v", tool.received)
	}
}

func TestMcpCallResolvesBySuffixWhenServerWrong(t *testing.T) {
	_, d, tool := newDispatchRegistry()
	res := d.Execute(context.Background(), map[string]any{
		"server": "typo", "tool": "create_issue",
		"arguments": map[string]any{"issueId": "A"},
	})
	if res.IsError || tool.received == nil {
		t.Errorf("misspelled server name should fall back to unique suffix match: %s", res.Output)
	}
}

func TestMcpCallAmbiguousSuffixErrors(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockDispatchTool{name: "mcp__linear__create_issue", server: "linear"})
	reg.Register(&mockDispatchTool{name: "mcp__jira__create_issue", server: "jira"})
	d := &McpCallTool{Registry: reg}
	res := d.Execute(context.Background(), map[string]any{
		"server": "nope", "tool": "create_issue", "arguments": map[string]any{},
	})
	if !res.IsError {
		t.Fatal("ambiguous suffix across two tools should error, not guess")
	}
	// The error message must list available tools so the model knows how to fix it
	if !strings.Contains(res.Output, "mcp__linear__create_issue") {
		t.Errorf("error message should list available tools: %s", res.Output)
	}
}

func TestMcpCallCoercesBeforeDispatch(t *testing.T) {
	_, d, tool := newDispatchRegistry()
	d.Execute(context.Background(), map[string]any{
		"server": "linear", "tool": "create_issue",
		"arguments": map[string]any{"issueId": float64(8891), "ports": []any{"1"}},
	})
	if tool.received["issueId"] != "8891" {
		t.Errorf("issueId should be coerced to string: %#v", tool.received["issueId"])
	}
	ports, _ := tool.received["ports"].([]any)
	if len(ports) != 1 || ports[0] != int64(1) {
		t.Errorf("ports elements should be coerced to integers: %#v", tool.received["ports"])
	}
}

func TestMcpCallPermissionContentNormalization(t *testing.T) {
	cases := []struct{ server, tool, want string }{
		{"linear", "mcp__linear__create_issue", "linear__create_issue"},
		{"linear", "create_issue", "linear__create_issue"},
		{"chrome-2", "mcp__chrome_2__click", "chrome_2__click"},
		// Short name and full name must produce the same content, otherwise rules will miss
		{"chrome-devtools", "click", "chrome_devtools__click"},
		{"chrome-devtools", "mcp__chrome_devtools__click", "chrome_devtools__click"},
	}
	for _, c := range cases {
		if got := McpCallPermissionContent(c.server, c.tool); got != c.want {
			t.Errorf("(%s,%s) got %q, want %q", c.server, c.tool, got, c.want)
		}
	}
}

// Search and dispatch are only exposed to the model in modes that need them.
// Under eager, all MCP tools are already in tools[]; there is nothing to search
// and no dispatch entry point needed, so sending both would only waste tokens.
func TestToolExposureByMode(t *testing.T) {
	build := func(mode McpLoadingMode) []string {
		reg := NewRegistry()
		reg.Register(&ToolSearchTool{Registry: reg})
		reg.Register(&McpCallTool{Registry: reg})
		mcpTool := &mockDispatchTool{name: "mcp__linear__create_issue", server: "linear", schema: testSchema}
		reg.Register(mcpTool)

		// Simulate the effect of mcp.ApplyMode to avoid a reverse dependency from tools to mcp
		reg.McpLoadingMode = mode
		eager := mode == McpLoadingEager
		mcpTool.SetDeferLoading(!eager)
		reg.ExposeToolSearch = !eager
		reg.ExposeMcpCall = mode == McpLoadingDispatch

		var names []string
		for _, s := range reg.GetAllSchemas("anthropic") {
			names = append(names, s["name"].(string))
		}
		sort.Strings(names)
		return names
	}

	cases := []struct {
		mode McpLoadingMode
		want []string
	}{
		{McpLoadingEager, []string{"mcp__linear__create_issue"}},
		{McpLoadingNative, []string{"ToolSearch", "mcp__linear__create_issue"}},
		{McpLoadingDispatch, []string{"McpCall", "ToolSearch"}},
	}
	for _, c := range cases {
		got := build(c.mode)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("mode %s: tools[] = %v, want %v", c.mode, got, c.want)
		}
	}
}

// When no MCP is connected, ApplyMode is never called and both flags stay off.
func TestToolExposureDefaultsOff(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&ToolSearchTool{Registry: reg})
	reg.Register(&McpCallTool{Registry: reg})
	if n := len(reg.GetAllSchemas("anthropic")); n != 0 {
		t.Errorf("neither tool should be exposed without MCP, got %d", n)
	}
}
