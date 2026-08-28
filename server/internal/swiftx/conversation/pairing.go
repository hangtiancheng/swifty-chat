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

package conversation

// Anthropic requires every tool_use to have a paired tool_result; a single
// missing pair causes the entire request to be rejected. Unpaired entries can
// reach the conversation history in several ways: the user interrupting mid
// tool execution, a session being restored from disk after a process exit, or
// interleaved concurrent writes. These placeholder messages fill the gaps; on
// seeing them the model should understand that the tool produced no output.
const (
	// InterruptedToolResult fills in a tool call that has no result. The tool
	// may never have started, or it may have been interrupted halfway through,
	// so the wording must not assert that it had no side effects.
	InterruptedToolResult = "Tool execution was interrupted. The tool may or may not have completed; verify before relying on its effects."
	// RejectedToolResult fills in a tool call the user explicitly refused to
	// authorize. In this case we can assert that nothing changed, and that must
	// be stated clearly; otherwise the model will assume the change took effect
	// and proceed accordingly.
	RejectedToolResult = "The user rejected this tool use. Nothing was changed (for file edits, the new content was NOT written)."
)

// EnsureToolPairing returns a copy of the messages with pairing repaired; the
// input is not modified.
//
// It does two things:
//   - appends an error-marked tool_result for any tool_use that lacks a result,
//     placed immediately after the call
//   - drops orphan tool_results whose matching tool_use cannot be found
//
// The call site runs before sending the request, so interruption, session
// restoration, and concurrent interleaving all need only this single fallback
// rather than a per-frontend implementation. The synthesized content is not
// written back to the conversation history: history should faithfully record
// what actually happened, while the fill-in exists only to make this one
// request valid.
func EnsureToolPairing(messages []Message) []Message {
	resolved := make(map[string]struct{})
	issued := make(map[string]struct{})
	for _, m := range messages {
		for _, tr := range m.ToolResults {
			resolved[tr.ToolUseID] = struct{}{}
		}
		for _, tu := range m.ToolUses {
			issued[tu.ToolUseID] = struct{}{}
		}
	}

	out := make([]Message, 0, len(messages))
	for _, m := range messages {
		// Orphan tool result: the matching call is not in history, so keeping it
		// would only cause the request to be rejected.
		if len(m.ToolResults) > 0 {
			kept := make([]ToolResultBlock, 0, len(m.ToolResults))
			for _, tr := range m.ToolResults {
				if _, ok := issued[tr.ToolUseID]; ok {
					kept = append(kept, tr)
				}
			}
			if len(kept) == 0 && m.Content == "" && len(m.ToolUses) == 0 {
				continue // the message is now an empty shell; drop it to preserve role alternation
			}
			m.ToolResults = kept
		}

		out = append(out, m)

		// Dangling tool call: append an error result right after it so the
		// tool_use and tool_result remain adjacent.
		var missing []ToolResultBlock
		for _, tu := range m.ToolUses {
			if _, ok := resolved[tu.ToolUseID]; ok {
				continue
			}
			missing = append(missing, ToolResultBlock{
				ToolUseID: tu.ToolUseID,
				Content:   InterruptedToolResult,
				IsError:   true,
			})
			resolved[tu.ToolUseID] = struct{}{}
		}
		if len(missing) > 0 {
			out = append(out, Message{Role: "user", ToolResults: missing})
		}
	}
	return out
}
