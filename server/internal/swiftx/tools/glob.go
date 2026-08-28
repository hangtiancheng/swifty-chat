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
	"path/filepath"
	"sort"
	"strings"
)

type GlobTool struct{}

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string { return GlobDescription }

func (t *GlobTool) Category() ToolCategory { return CategoryRead }

func (t *GlobTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Glob pattern to match (e.g. '**/*.py')"},
				"path":    map[string]any{"type": "string", "description": "Base directory to search from", "default": "."},
			},
			"required": []string{"pattern"},
		},
	}
}

func (t *GlobTool) Execute(_ context.Context, args map[string]any) ToolResult {
	pattern, _ := args["pattern"].(string)
	basePath, _ := args["path"].(string)
	if basePath == "" {
		basePath = "."
	}
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required", IsError: true}
	}

	info, err := os.Stat(basePath)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: path not found: %s", basePath), IsError: true}
	}
	if err != nil || !info.IsDir() {
		return ToolResult{Output: fmt.Sprintf("Error: path not found: %s", basePath), IsError: true}
	}

	// Recognize doublestar `**/` prefix and treat it as "match basePattern at
	// any depth". Go's filepath.Match doesn't understand `**`; without this,
	// the most common LLM-issued patterns like `**/*.go` silently match nothing.
	recursive := false
	basePattern := pattern
	for strings.HasPrefix(basePattern, "**/") {
		basePattern = basePattern[3:]
		recursive = true
	}

	var matches []string
	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if SkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(basePath, path)
		matched := false
		if recursive {
			// `**/<basePattern>` — match basePattern against base name at any depth.
			matched, _ = filepath.Match(basePattern, filepath.Base(path))
		} else {
			matched, _ = filepath.Match(pattern, filepath.Base(path))
			if !matched {
				matched, _ = filepath.Match(pattern, rel)
			}
		}
		if matched {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %s", err), IsError: true}
	}

	// Sort by modification time descending — most recently modified first.
	sort.Slice(matches, func(i, j int) bool {
		fi, ei := os.Stat(filepath.Join(basePath, matches[i]))
		fj, ej := os.Stat(filepath.Join(basePath, matches[j]))
		if ei != nil || ej != nil {
			return matches[i] < matches[j]
		}
		return fi.ModTime().After(fj.ModTime())
	})
	if len(matches) == 0 {
		return ToolResult{Output: "No files matched the pattern."}
	}
	return ToolResult{Output: strings.Join(matches, "\n")}
}
