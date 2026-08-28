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
	"sync"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// toolBatch is the output of partitionToolCalls: a group of tool calls
// together with a flag indicating whether they may run concurrently.
type toolBatch struct {
	concurrent bool
	calls      []toolCallEntry
}

type toolCallEntry struct {
	tc    toolCallInfo
	index int // position in the original toolCalls list, preserving result order
}

type toolCallInfo struct {
	toolID    string
	toolName  string
	arguments map[string]any
}

// StreamingExecutor executes tool calls in batches based on safety:
// consecutive read-only tools (category == "read") are merged into a single
// batch that runs concurrently, while write/command tools each occupy their
// own batch and run serially.
type StreamingExecutor struct {
	registry *tools.Registry
	eventCh  chan AgentEvent

	mu      sync.Mutex
	calls   []toolCallInfo
	results []toolExecResult
}

func NewStreamingExecutor(registry *tools.Registry, eventCh chan AgentEvent) *StreamingExecutor {
	return &StreamingExecutor{
		registry: registry,
		eventCh:  eventCh,
	}
}

// Submit collects a tool call for later execution (it does not run immediately).
func (se *StreamingExecutor) Submit(tc toolCallInfo) {
	se.mu.Lock()
	se.calls = append(se.calls, tc)
	se.mu.Unlock()
}

// ExecuteAll partitions the collected tool calls into batches and runs them in
// order: read-only batches run concurrently, write/command batches run serially.
// Results are returned in the original submission order.
func (se *StreamingExecutor) ExecuteAll(ctx context.Context, agent *Agent) []toolExecResult {
	se.mu.Lock()
	calls := append([]toolCallInfo(nil), se.calls...)
	se.mu.Unlock()

	results := make([]toolExecResult, len(calls))

	// Record the original index of each call so results can be placed back
	// in order after batching.
	var entries []toolCallEntry
	for i, c := range calls {
		entries = append(entries, toolCallEntry{tc: c, index: i})
	}

	batches := partitionToolCalls(entries, se.registry)

	for _, batch := range batches {
		if batch.concurrent && len(batch.calls) > 1 {
			// Read-only batch: execute concurrently
			var wg sync.WaitGroup
			for _, entry := range batch.calls {
				wg.Add(1)
				go func(e toolCallEntry) {
					defer wg.Done()
					r := agent.executeSingleTool(ctx, se.eventCh, e.tc)
					se.mu.Lock()
					results[e.index] = r
					se.mu.Unlock()
				}(entry)
			}
			wg.Wait()
		} else {
			// Write/command batch: execute serially
			for _, entry := range batch.calls {
				r := agent.executeSingleTool(ctx, se.eventCh, entry.tc)
				results[entry.index] = r
			}
		}
	}

	se.results = results
	return results
}

// partitionToolCalls groups adjacent tool calls into batches: consecutive
// concurrency-safe calls form a single concurrent batch; all others each
// occupy their own serial batch.
//
// Safety is determined by the actual arguments of each invocation, not merely
// by the tool category. Both ls and rm are Bash, but the former can run
// concurrently with ReadFile while the latter must run exclusively.
func partitionToolCalls(entries []toolCallEntry, registry *tools.Registry) []toolBatch {
	var batches []toolBatch
	for _, entry := range entries {
		tool := registry.Get(entry.tc.toolName)
		safe := tool != nil && tools.IsConcurrencySafe(tool, entry.tc.arguments)

		if safe && len(batches) > 0 && batches[len(batches)-1].concurrent {
			batches[len(batches)-1].calls = append(batches[len(batches)-1].calls, entry)
		} else {
			batches = append(batches, toolBatch{
				concurrent: safe,
				calls:      []toolCallEntry{entry},
			})
		}
	}
	return batches
}

// HasPending reports whether any tool calls have been submitted.
func (se *StreamingExecutor) HasPending() bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	return len(se.calls) > 0
}

// Reset clears the executor state for the next turn.
func (se *StreamingExecutor) Reset() {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.calls = nil
	se.results = nil
}
