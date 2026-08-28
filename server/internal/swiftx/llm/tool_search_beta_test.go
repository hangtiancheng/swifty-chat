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

package llm

import "testing"

// Beta header gating: only send it when a tool actually carries defer_loading.
//
// The official-endpoint path cannot be verified against third-party endpoints
// like MiniMax in production, so this test pins down exactly what the request
// should look like: a missing header causes the server to reject defer_loading
// outright; an extra header causes unrecognized endpoints to reject as well.
// Both directions are hard failures.
func TestNeedsToolSearchBeta(t *testing.T) {
	cases := []struct {
		desc  string
		tools []map[string]any
		want  bool
	}{
		{"no tools", nil, false},
		{"no deferred tools", []map[string]any{{"name": "Bash"}, {"name": "ToolSearch"}}, false},
		{
			"one tool with defer_loading",
			[]map[string]any{{"name": "Bash"}, {"name": "mcp__linear__x", "defer_loading": true}},
			true,
		},
		{
			"defer_loading set to false does not count",
			[]map[string]any{{"name": "mcp__linear__x", "defer_loading": false}},
			false,
		},
	}
	for _, c := range cases {
		if got := needsToolSearchBeta(c.tools); got != c.want {
			t.Errorf("%s: got %v, want %v", c.desc, got, c.want)
		}
	}
}
