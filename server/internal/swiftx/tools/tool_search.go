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
)

type ToolSearchTool struct {
	Registry *Registry
	Protocol string
}

func (t *ToolSearchTool) Name() string { return ToolSearchToolName }

func (t *ToolSearchTool) Description() string {
	return `Search for and load additional tools that are not immediately available. Some tools are deferred (not loaded by default) to save context space. Use this tool to discover and load them.

Query forms:
- "select:ToolName,AnotherTool" — fetch exact tools by name
- "keyword search" — keyword search, returns up to max_results matches

When you need a tool that isn't in your current tool list, use this to find it.`
}

func (t *ToolSearchTool) Category() ToolCategory { return CategoryRead }

func (t *ToolSearchTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": `Query to find deferred tools. Use "select:Name1,Name2" for direct selection, or keywords to search.`,
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum results to return (default: 5)",
					"default":     5,
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *ToolSearchTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return ToolResult{Output: "Error: query is required", IsError: true}
	}

	maxResults := intArg(args, "max_results", 5)
	if maxResults < 1 {
		maxResults = 5
	}
	if maxResults > 20 {
		maxResults = 20
	}

	var schemas []map[string]any

	if after, ok := strings.CutPrefix(query, "select:"); ok {
		names := strings.Split(after, ",")
		for i := range names {
			names[i] = strings.TrimSpace(names[i])
		}
		schemas = t.Registry.FindDeferredByNames(names, t.Protocol)
	} else {
		schemas = t.Registry.SearchDeferred(query, maxResults, t.Protocol)
	}

	if len(schemas) == 0 {
		deferredNames := t.Registry.GetDeferredToolNames()
		if len(deferredNames) == 0 {
			return ToolResult{
				Output: fmt.Sprintf("No deferred tools available for query %q.", query),
			}
		}
		return ToolResult{
			Output: fmt.Sprintf("No matching deferred tools found for query %q. Available deferred tools: %s",
				query, strings.Join(deferredNames, ", ")),
		}
	}

	// Non-MCP deferred tools have no McpCall entry point; mark them as
	// discovered so they appear in the next round's tools[]
	var mcpNames []string
	for _, s := range schemas {
		name, ok := s["name"].(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(name, MCPToolPrefix) {
			mcpNames = append(mcpNames, name)
		} else {
			t.Registry.MarkDiscovered(name)
		}
	}

	// Official endpoint: return tool_reference blocks so the server expands
	// the schema into context. The tools array stays unchanged, preserving
	// the cache prefix.
	if len(mcpNames) > 0 && t.Registry.McpLoadingMode == McpLoadingNative && !isOpenAIProtocol(t.Protocol) {
		blocks := make([]map[string]any, 0, len(mcpNames))
		for _, name := range mcpNames {
			blocks = append(blocks, map[string]any{
				"type":      "tool_reference",
				"tool_name": name,
			})
		}
		return ToolResult{
			Output: fmt.Sprintf("Loaded %d tool(s): %s. You can call them directly now.",
				len(mcpNames), strings.Join(mcpNames, ", ")),
			ContentBlocks: blocks,
		}
	}

	// Other endpoints: show the raw schema to the model and route calls
	// through McpCall. This text is appended to messages and does not
	// affect the cache prefix.
	suffix := ""
	if len(mcpNames) > 0 {
		suffix = "\n\nTo invoke any of the tools above, call McpCall with that tool's " +
			"full name and an `arguments` object matching its input_schema exactly, " +
			"using the same JSON types."
	}
	schemasJSON, _ := json.MarshalIndent(schemas, "", "  ")
	return ToolResult{
		Output: fmt.Sprintf("Found %d tool(s). Their full schemas are below:\n\n%s%s",
			len(schemas), string(schemasJSON), suffix),
	}
}
