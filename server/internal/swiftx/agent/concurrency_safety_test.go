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

package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// plainT is a stub tool used solely for batch-partition tests, with a configurable category.
type plainT struct {
	n   string
	cat tools.ToolCategory
}

func (t *plainT) Name() string                 { return t.n }
func (t *plainT) Description() string          { return t.n }
func (t *plainT) Category() tools.ToolCategory { return t.cat }
func (t *plainT) Schema() map[string]any       { return map[string]any{"name": t.n} }
func (t *plainT) Execute(context.Context, map[string]any) tools.ToolResult {
	return tools.ToolResult{}
}

// Concurrency safety is determined by the actual arguments of each invocation,
// not merely by the tool category.
//
// Both ls and rm are Bash, but the former mutates no external state and can run
// concurrently with ReadFile, while the latter must run exclusively to preserve
// the model-intended execution order.
func TestBashConcurrencySafetyByCommand(t *testing.T) {
	bash := &tools.BashTool{}

	safe := []string{"ls", "ls -la", "cat a.txt", "git status", "wc -l f", "pwd"}
	for _, cmd := range safe {
		if !tools.IsConcurrencySafe(bash, map[string]any{"command": cmd}) {
			t.Errorf("%q should be concurrency-safe", cmd)
		}
	}

	unsafe := []string{
		"rm -rf build", "mv a b", "npm install", "git commit -m x",
		"echo hi > f", "ls | wc -l", "ls; rm x", "ls && rm x",
		"echo $(rm x)", "ls `rm x`",
	}
	for _, cmd := range unsafe {
		if tools.IsConcurrencySafe(bash, map[string]any{"command": cmd}) {
			t.Errorf("%q should not be concurrency-safe", cmd)
		}
	}
}

// Missing or malformed arguments are treated as unsafe; prefer serial execution over guessing.
func TestBashConcurrencySafetyBadArgs(t *testing.T) {
	bash := &tools.BashTool{}
	for _, args := range []map[string]any{
		{},
		{"command": nil},
		{"command": 123},
	} {
		if tools.IsConcurrencySafe(bash, args) {
			t.Errorf("args=%v should not be concurrency-safe", args)
		}
	}
}

// Tools that do not implement the optional interface fall back to category-based safety.
func TestConcurrencySafetyFallsBackToCategory(t *testing.T) {
	cases := []struct {
		name string
		tool tools.Tool
		want bool
	}{
		{"read", &plainT{n: "ReadFile", cat: tools.CategoryRead}, true},
		{"write", &plainT{n: "WriteFile", cat: tools.CategoryWrite}, false},
		{"command", &plainT{n: "Other", cat: tools.CategoryCommand}, false},
	}
	for _, c := range cases {
		if got := tools.IsConcurrencySafe(c.tool, nil); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// Read-only Bash commands must be grouped into the same concurrent batch as ReadFile.
func TestReadOnlyBashBatchesWithReadTools(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&plainT{n: "ReadFile", cat: tools.CategoryRead})
	reg.Register(&plainT{n: "Grep", cat: tools.CategoryRead})
	reg.Register(&tools.BashTool{})

	entries := []toolCallEntry{
		{tc: toolCallInfo{toolID: "a", toolName: "ReadFile"}, index: 0},
		{tc: toolCallInfo{toolID: "b", toolName: "Bash",
			arguments: map[string]any{"command": "git status"}}, index: 1},
		{tc: toolCallInfo{toolID: "c", toolName: "Grep"}, index: 2},
	}

	batches := partitionToolCalls(entries, reg)
	if len(batches) != 1 || !batches[0].concurrent || len(batches[0].calls) != 3 {
		t.Fatalf("expected a single concurrent batch of 3 calls, got %s", describe(batches))
	}
}

// A mutating Bash command must break the batch; calls before and after form separate batches.
func TestMutatingBashBreaksTheBatch(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&plainT{n: "ReadFile", cat: tools.CategoryRead})
	reg.Register(&plainT{n: "Grep", cat: tools.CategoryRead})
	reg.Register(&tools.BashTool{})

	entries := []toolCallEntry{
		{tc: toolCallInfo{toolID: "a", toolName: "ReadFile"}, index: 0},
		{tc: toolCallInfo{toolID: "b", toolName: "Bash",
			arguments: map[string]any{"command": "rm -rf build"}}, index: 1},
		{tc: toolCallInfo{toolID: "c", toolName: "Grep"}, index: 2},
	}

	batches := partitionToolCalls(entries, reg)
	want := "[concurrent:a] [serial:b] [concurrent:c]"
	if got := describe(batches); got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func describe(batches []toolBatch) string {
	out := ""
	for i, b := range batches {
		if i > 0 {
			out += " "
		}
		kind := "serial"
		if b.concurrent {
			kind = "concurrent"
		}
		ids := ""
		for j, c := range b.calls {
			if j > 0 {
				ids += ","
			}
			ids += c.tc.toolID
		}
		out += fmt.Sprintf("[%s:%s]", kind, ids)
	}
	return out
}
