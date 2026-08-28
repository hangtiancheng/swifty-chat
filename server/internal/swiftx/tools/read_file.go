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
	"fmt"
	"os"
	"strings"
)

type ReadFileTool struct {
	FileStateCache *FileStateCache
}

func (t *ReadFileTool) Name() string        { return "ReadFile" }
func (t *ReadFileTool) Description() string { return ReadFileDescription }

func (t *ReadFileTool) Category() ToolCategory { return CategoryRead }

func (t *ReadFileTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Absolute or relative path to the file to read"},
				"offset":    map[string]any{"type": "integer", "description": "Line offset to start reading from (0-based)", "default": 0},
				"limit":     map[string]any{"type": "integer", "description": "Maximum number of lines to read", "default": 2000},
			},
			"required": []string{"file_path"},
		},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, args map[string]any) ToolResult {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return ToolResult{Output: "Error: file_path is required", IsError: true}
	}

	offset := intArg(args, "offset", 0)
	limit := intArg(args, "limit", 2000)

	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", filePath), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %s", err), IsError: true}
	}
	if info.IsDir() {
		return ToolResult{Output: fmt.Sprintf("Error: not a file: %s", filePath), IsError: true}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %s", err), IsError: true}
	}

	lines := strings.Split(string(data), "\n")
	if offset >= len(lines) {
		return ToolResult{Output: ""}
	}
	end := min(offset+limit, len(lines))
	selected := lines[offset:end]

	// Record file state for read-before-edit enforcement
	if t.FileStateCache != nil {
		t.FileStateCache.Record(filePath, info.ModTime().UnixMilli())
	}

	var sb strings.Builder
	for i, line := range selected {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "%d\t%s", i+offset+1, line)
	}
	return ToolResult{Output: sb.String()}
}

func intArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return def
}
