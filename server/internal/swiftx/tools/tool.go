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
	"sort"
	"strings"
)

var SkipDirs = map[string]bool{
	".git": true, ".venv": true, "node_modules": true,
	"__pycache__": true, ".tox": true, ".mypy_cache": true,
}

// MaxOutputChars is the spill threshold applied before a single tool result
// enters the conversation history: content exceeding this character count is
// written to disk, leaving only a preview and the file path in history. Set
// to 50000 rather than a smaller value so the model can see enough content
// in one pass without needing an extra ReadFile round-trip.
const MaxOutputChars = 50000

type ToolResult struct {
	Output  string
	IsError bool
	// ContentBlocks carries structured content blocks instead of plain text
	// for tool results. Currently only ToolSearch on the official Anthropic
	// endpoint uses this: it returns tool_reference blocks that the server
	// expands into context. When this field is populated, Output still holds
	// the equivalent text for TUI and log display.
	ContentBlocks []map[string]any
}

// McpLoadingMode determines how MCP tools enter the context. It is written to
// the Registry by internal/mcp after connecting to servers. It lives here
// rather than in internal/mcp because Registry holds it, and internal/mcp
// depends on internal/tools — a reverse reference would create a cycle.
type McpLoadingMode string

const (
	// McpLoadingEager: total schema size is under 10% of context; load all
	// tools into tools[] with no deferral.
	McpLoadingEager McpLoadingMode = "eager"
	// McpLoadingNative: official endpoint. Tools stay in tools[] with
	// defer_loading but the server hides them from the model; ToolSearch
	// returns tool_reference so the server expands the schema.
	McpLoadingNative McpLoadingMode = "native"
	// McpLoadingDispatch: other endpoints do not support defer_loading;
	// MCP tools never enter tools[] and calls go through McpCall.
	McpLoadingDispatch McpLoadingMode = "dispatch"
)

// MCPTool exposes additional capabilities of MCP tool wrappers to the dispatch
// and routing logic. A structured interface is used instead of a direct
// reference to internal/mcp, again to avoid a circular dependency.
type MCPTool interface {
	Tool
	MCPServerName() string
	MCPInputSchema() map[string]any
	SetDeferLoading(bool)
}

// ToolSearchToolName is the tool-search tool's name, used when the registry
// filters tools by mode.
const ToolSearchToolName = "ToolSearch"

type ToolCategory string

const (
	CategoryRead    ToolCategory = "read"
	CategoryWrite   ToolCategory = "write"
	CategoryCommand ToolCategory = "command"
)

type Tool interface {
	Name() string
	Description() string
	Category() ToolCategory
	Schema() map[string]any
	Execute(ctx context.Context, args map[string]any) ToolResult
}

// DeferrableTool lets a tool declare whether it should be lazily loaded.
// Deferred tools do not appear in the initial tool list; the model must first
// use ToolSearch to retrieve the schema before invoking them.
//
// Only MCP tools implement this interface. MCP servers are configured
// per-project and can expose dozens of tools with lengthy schemas; including
// all of them in the initial tool list would consume a large portion of the
// context, and most tools are unused in any given session. Built-in tools are
// a fixed, manageable set — hiding them would only force the model through an
// extra ToolSearch round-trip, so they are never deferred and always expose
// their full schema.
type DeferrableTool interface {
	ShouldDefer() bool
}

// ConcurrencySafeTool lets a tool decide concurrency safety based on the
// actual arguments of a specific invocation.
//
// Tools that do not implement this interface fall back to category-based
// rules: read-only tools may run concurrently; write and command tools may not.
// Currently only Bash implements it, because whether a command is read-only
// depends on the command itself — ls and rm are both Bash but differ entirely
// in concurrency safety.
type ConcurrencySafeTool interface {
	IsConcurrencySafe(args map[string]any) bool
}

// IsConcurrencySafe reports whether a tool invocation can run concurrently
// with other invocations.
//
// If the tool implements ConcurrencySafeTool, its verdict is used; otherwise
// the decision falls back to the tool category.
func IsConcurrencySafe(t Tool, args map[string]any) bool {
	if cs, ok := t.(ConcurrencySafeTool); ok {
		return cs.IsConcurrencySafe(args)
	}
	return t.Category() == CategoryRead
}

type Registry struct {
	tools           map[string]Tool
	discoveredTools map[string]bool
	// McpLoadingMode is written by mcp.DecideAndApply after connecting to
	// servers. Without MCP it stays eager, which is equivalent to no deferral.
	McpLoadingMode McpLoadingMode

	// ExposeToolSearch / ExposeMcpCall control whether these two tools are
	// sent to the model, computed once by mcp.ApplyMode at session start.
	// They are not recomputed each turn based on "are there still deferred
	// tools": tools may be disabled at runtime, and recomputing would remove
	// a tool mid-session — that is an array change that breaks the cache prefix.
	ExposeToolSearch bool
	ExposeMcpCall    bool
}

func NewRegistry() *Registry {
	return &Registry{
		tools:           make(map[string]Tool),
		discoveredTools: make(map[string]bool),
		McpLoadingMode:  McpLoadingEager,
	}
}

func (r *Registry) MarkDiscovered(name string) {
	r.discoveredTools[name] = true
}

func (r *Registry) IsDiscovered(name string) bool {
	return r.discoveredTools[name]
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

func (r *Registry) ListTools() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

func isDeferred(t Tool) bool {
	if dt, ok := t.(DeferrableTool); ok {
		return dt.ShouldDefer()
	}
	return false
}

func isOpenAIProtocol(protocol string) bool {
	return protocol == "openai" || protocol == "openai-compat"
}

// GetAllSchemas builds the tool list to send to the model for this turn.
//
// It iterates over sorted tool names rather than the map directly. Go map iteration order is
// random; without sorting, the same set of tools would serialize in a different order each time.
// Since the tool list is rendered after the system prompt and before messages, any order change
// invalidates the byte prefix and busts the conversation history cache that follows — the cost is
// the same as actually adding a tool, even though the content is unchanged.
func (r *Registry) GetAllSchemas(protocol string) []map[string]any {
	// Official endpoint uses native deferral: tools stay in tools[] with
	// defer_loading and the server decides visibility. This way, even when
	// new tools are discovered, the tools array bytes do not change and the
	// prompt cache prefix is preserved. Other endpoints must hide deferred
	// tools entirely and rely on McpCall.
	native := r.McpLoadingMode == McpLoadingNative && !isOpenAIProtocol(protocol)
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	schemas := make([]map[string]any, 0, len(r.tools))
	for _, name := range names {
		t := r.tools[name]
		// Search and dispatch are only exposed in modes that need them. Under eager there are
		// no deferred tools to search and no dispatch needed; sending both would only waste
		// tokens and might tempt the model into a needless detour.
		if name == ToolSearchToolName && !r.ExposeToolSearch {
			continue
		} else if name == McpCallToolName && !r.ExposeMcpCall {
			continue
		}
		deferred := isDeferred(t) && !r.discoveredTools[name]
		if deferred && !native {
			continue
		}
		base := t.Schema()
		if isOpenAIProtocol(protocol) {
			schemas = append(schemas, map[string]any{
				"type":        "function",
				"name":        base["name"],
				"description": base["description"],
				"parameters":  base["input_schema"],
			})
		} else {
			if deferred {
				withFlag := make(map[string]any, len(base)+1)
				for k, v := range base {
					withFlag[k] = v
				}
				withFlag["defer_loading"] = true
				base = withFlag
			}
			schemas = append(schemas, base)
		}
	}
	return schemas
}

// GetDeferredToolNames returns the names of deferred tools not yet discovered, in lexicographic
// order. Sorting is not cosmetic: tools is a map, so without sorting each call would produce a
// different order, making it impossible for callers to detect whether the pool actually changed
// by comparing lists.
func (r *Registry) GetDeferredToolNames() []string {
	var names []string
	for _, t := range r.tools {
		if isDeferred(t) && !r.discoveredTools[t.Name()] {
			names = append(names, t.Name())
		}
	}
	sort.Strings(names)
	return names
}

func (r *Registry) GetDeferredTools() []Tool {
	var result []Tool
	for _, t := range r.tools {
		if isDeferred(t) {
			result = append(result, t)
		}
	}
	return result
}

func (r *Registry) SearchDeferred(query string, maxResults int, protocol string) []map[string]any {
	query = strings.ToLower(query)
	var matches []map[string]any
	for _, t := range r.tools {
		if !isDeferred(t) {
			continue
		}
		name := strings.ToLower(t.Name())
		desc := strings.ToLower(t.Description())
		if strings.Contains(name, query) || strings.Contains(desc, query) {
			base := t.Schema()
			if isOpenAIProtocol(protocol) {
				matches = append(matches, map[string]any{
					"type":        "function",
					"name":        base["name"],
					"description": base["description"],
					"parameters":  base["input_schema"],
				})
			} else {
				matches = append(matches, base)
			}
			if len(matches) >= maxResults {
				break
			}
		}
	}
	return matches
}

func (r *Registry) FindDeferredByNames(names []string, protocol string) []map[string]any {
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[strings.ToLower(n)] = true
	}
	var matches []map[string]any
	for _, t := range r.tools {
		if nameSet[strings.ToLower(t.Name())] {
			base := t.Schema()
			if isOpenAIProtocol(protocol) {
				matches = append(matches, map[string]any{
					"type":        "function",
					"name":        base["name"],
					"description": base["description"],
					"parameters":  base["input_schema"],
				})
			} else {
				matches = append(matches, base)
			}
		}
	}
	return matches
}

type DefaultTools struct {
	Registry  *Registry
	WriteFile *WriteFileTool
	EditFile  *EditFileTool
}

func CreateDefaultRegistry() *Registry {
	dt := CreateDefaultTools()
	return dt.Registry
}

func CreateDefaultToolsWithWorkDir(workDir string) DefaultTools {
	fsc := NewFileStateCache()
	wf := &WriteFileTool{FileStateCache: fsc}
	ef := &EditFileTool{FileStateCache: fsc}
	reg := NewRegistry()
	reg.Register(&ReadFileTool{FileStateCache: fsc})
	reg.Register(wf)
	reg.Register(ef)
	reg.Register(&BashTool{WorkDir: workDir})
	reg.Register(&GlobTool{})
	reg.Register(&GrepTool{})
	return DefaultTools{Registry: reg, WriteFile: wf, EditFile: ef}
}

func CreateDefaultTools() DefaultTools {
	return CreateDefaultToolsWithWorkDir("")
}
