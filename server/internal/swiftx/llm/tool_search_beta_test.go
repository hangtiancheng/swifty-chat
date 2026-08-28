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
