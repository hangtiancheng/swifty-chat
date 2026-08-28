package llm

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// Cache breakpoint placement.
//
// The marker must land on the last non-deferred tool. A tool carrying both defer_loading and
// cache_control causes the official endpoint to reject the entire request (400). MCP tools are
// registered after built-in tools, so after sorting the tail is often a deferred tool — simply
// marking the last element is not safe.
func TestMarkToolsForCache(t *testing.T) {
	plain := func(name string) anthropic.ToolUnionParam {
		return anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{Name: name}}
	}
	deferred := func(name string) anthropic.ToolUnionParam {
		return anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:         name,
			DeferLoading: param.NewOpt(true),
		}}
	}
	// CacheControl is a value type; the zero value means unmarked. Comparing against the zero
	// value directly is the most reliable check.
	var unmarked anthropic.CacheControlEphemeralParam
	marked := func(tools []anthropic.ToolUnionParam) []string {
		var out []string
		for _, u := range tools {
			if u.OfTool == nil || u.OfTool.CacheControl == unmarked {
				continue
			}
			out = append(out, u.OfTool.Name)
		}
		return out
	}

	t.Run("scans backward when tail is deferred", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{
			plain("ReadFile"), plain("WriteFile"), plain("ToolSearch"),
			deferred("mcp__linear__create_issue"), deferred("mcp__sentry__resolve"),
		}
		markToolsForCache(tools)
		got := marked(tools)
		if len(got) != 1 || got[0] != "ToolSearch" {
			t.Fatalf("expected ToolSearch to be marked, got %v", got)
		}
	})

	t.Run("marks last tool when none are deferred", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{plain("ReadFile"), plain("Bash")}
		markToolsForCache(tools)
		if got := marked(tools); len(got) != 1 || got[0] != "Bash" {
			t.Fatalf("expected Bash to be marked, got %v", got)
		}
	})

	t.Run("skips deferred tools interleaved in the middle", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{
			plain("Bash"), deferred("mcp__a__x"), plain("Grep"), deferred("mcp__z__y"),
		}
		markToolsForCache(tools)
		if got := marked(tools); len(got) != 1 || got[0] != "Grep" {
			t.Fatalf("expected Grep to be marked, got %v", got)
		}
	})

	t.Run("marks nothing when all tools are deferred", func(t *testing.T) {
		// The official endpoint requires at least one non-deferred tool. In practice built-in
		// tools are never deferred, so this is a defensive branch: better to skip caching than
		// to send a request that will be rejected with 400.
		tools := []anthropic.ToolUnionParam{deferred("mcp__a__x"), deferred("mcp__b__y")}
		markToolsForCache(tools)
		if got := marked(tools); len(got) != 0 {
			t.Fatalf("expected no tools to be marked, got %v", got)
		}
	})
}
