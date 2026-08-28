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

package hooks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEvaluateConditionLeafOps(t *testing.T) {
	ctx := HookContext{
		EventName: EventPreToolUse,
		ToolName:  "Bash",
		FilePath:  "src/foo.go",
		ToolArgs:  map[string]any{"command": "rm -rf /"},
	}
	cases := map[string]bool{
		`tool == "Bash"`:                            true,
		`tool == "Read"`:                            false,
		`tool != "Read"`:                            true,
		`event =~ /^pre_/`:                          true,
		`args.command =~ /rm -rf/`:                  true,
		`file_path =* "src/*.go"`:                   true,
		`file_path =* "src/*.py"`:                   false,
		`tool == "Bash" && file_path =* "src/*.go"`: true,
		`tool == "Bash" && file_path =* "src/*.py"`: false,
		`tool == "Read" || tool == "Bash"`:          true,
		`tool == "Read" || tool == "Write"`:         false,
		`!(tool == "Read")`:                         true, // ! before parens — falls through evaluateLeaf
		`!tool == "Read"`:                           true, // ! applied to leaf
	}
	for cond, want := range cases {
		if got := evaluateCondition(cond, ctx); got != want {
			t.Errorf("evaluateCondition(%q) = %v, want %v", cond, got, want)
		}
	}
}

func TestRunPreToolHooksReject(t *testing.T) {
	eng := NewEngine()
	eng.LoadHooks([]Hook{{
		ID:        "block-rm-rf",
		Event:     EventPreToolUse,
		Condition: `tool == "Bash" && args.command =~ /rm -rf/`,
		Action:    Action{Type: ActionPrompt, Message: "destructive command blocked"},
		Reject:    true,
	}})

	ctx := HookContext{
		EventName: EventPreToolUse,
		ToolName:  "Bash",
		ToolArgs:  map[string]any{"command": "rm -rf /tmp/x"},
	}
	rejected, msg := eng.RunPreToolHooks(ctx)
	if !rejected {
		t.Fatal("expected rejection")
	}
	if !strings.Contains(msg, "destructive command blocked") {
		t.Errorf("unexpected reject message: %q", msg)
	}
}

func TestRunPreToolHooksAllowsWhenConditionFails(t *testing.T) {
	eng := NewEngine()
	eng.LoadHooks([]Hook{{
		ID:        "block-go",
		Event:     EventPreToolUse,
		Condition: `file_path =* "**/*.go"`,
		Action:    Action{Type: ActionPrompt, Message: "blocked"},
		Reject:    true,
	}})
	rejected, _ := eng.RunPreToolHooks(HookContext{
		EventName: EventPreToolUse,
		ToolName:  "WriteFile",
		FilePath:  "src/foo.py",
	})
	if rejected {
		t.Fatal("expected allow for non-matching path")
	}
}

func TestHookOnceOnlyFiresOnce(t *testing.T) {
	eng := NewEngine()
	eng.LoadHooks([]Hook{{
		ID:     "greet",
		Event:  EventSessionStart,
		Action: Action{Type: ActionPrompt, Message: "hello"},
		Once:   true,
	}})

	res1 := eng.RunHooks(HookContext{EventName: EventSessionStart})
	res2 := eng.RunHooks(HookContext{EventName: EventSessionStart})
	if len(res1) != 1 {
		t.Errorf("first run should produce 1 result, got %d", len(res1))
	}
	if len(res2) != 0 {
		t.Errorf("second run should produce 0 results (once), got %d", len(res2))
	}
}

func TestHookHTTPAction(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Method != "POST" {
			t.Errorf("want POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing JSON content-type")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	eng := NewEngine()
	eng.LoadHooks([]Hook{{
		ID:    "notify",
		Event: EventPostToolUse,
		Action: Action{
			Type: ActionHTTP,
			URL:  server.URL,
		},
	}})
	results := eng.RunHooks(HookContext{EventName: EventPostToolUse, ToolName: "Bash"})
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected one successful HTTP result, got %#v", results)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("server expected 1 hit, got %d", hits)
	}
}

func TestHookAsyncIsNonBlocking(t *testing.T) {
	eng := NewEngine()
	eng.LoadHooks([]Hook{{
		ID:    "slow",
		Event: EventTurnEnd,
		Async: true,
		Action: Action{
			Type:    ActionCommand,
			Command: "sleep 0.2",
		},
	}})
	start := time.Now()
	res := eng.RunHooks(HookContext{EventName: EventTurnEnd})
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("async hook blocked the caller for %v", elapsed)
	}
	if len(res) != 1 || res[0].Output != "(async)" {
		t.Errorf("expected async stub result, got %#v", res)
	}
}

func TestHookOnErrorReject(t *testing.T) {
	eng := NewEngine()
	eng.LoadHooks([]Hook{{
		ID:      "fail",
		Event:   EventPreToolUse,
		OnError: "reject",
		Action: Action{
			Type:    ActionCommand,
			Command: "exit 7",
		},
	}})
	rejected, msg := eng.RunPreToolHooks(HookContext{EventName: EventPreToolUse, ToolName: "Bash"})
	if !rejected {
		t.Fatal("expected reject on command failure with on_error=reject")
	}
	_ = msg
}

func TestValidateCatchesMissingFields(t *testing.T) {
	cases := []struct {
		name string
		hook Hook
		want string // substring that must appear in the error
	}{
		{
			name: "command missing command field",
			hook: Hook{ID: "no-cmd", Event: EventPreToolUse, Action: Action{Type: ActionCommand}},
			want: "action.command must be non-empty",
		},
		{
			name: "prompt missing message",
			hook: Hook{ID: "no-msg", Event: EventSessionStart, Action: Action{Type: ActionPrompt}},
			want: "action.message must be non-empty",
		},
		{
			name: "http missing url",
			hook: Hook{ID: "no-url", Event: EventPostToolUse, Action: Action{Type: ActionHTTP}},
			want: "action.url must be non-empty",
		},
		{
			name: "http with malformed url",
			hook: Hook{ID: "bad-url", Event: EventPostToolUse, Action: Action{Type: ActionHTTP, URL: "not-a-url"}},
			want: "action.url must be a valid http(s) URL",
		},
		{
			name: "unknown event",
			hook: Hook{ID: "unknown-evt", Event: "made_up_event", Action: Action{Type: ActionPrompt, Message: "hi"}},
			want: `unknown event "made_up_event"`,
		},
		{
			name: "unknown action type",
			hook: Hook{ID: "unknown-act", Event: EventPreToolUse, Action: Action{Type: "weird"}},
			want: `unknown action.type "weird"`,
		},
		{
			name: "missing action type",
			hook: Hook{ID: "no-type", Event: EventPreToolUse, Action: Action{Command: "echo"}},
			want: "action.type is required",
		},
		{
			name: "negative timeout",
			hook: Hook{ID: "neg-to", Event: EventPostToolUse, Action: Action{Type: ActionCommand, Command: "echo ok", Timeout: -time.Second}},
			want: "action.timeout must be >= 0",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate([]Hook{c.hook})
			if err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected error to contain %q, got %q", c.want, err.Error())
			}
		})
	}
}

func TestValidateAggregatesAllErrors(t *testing.T) {
	hooks := []Hook{
		{ID: "bad1", Event: "nope", Action: Action{Type: ActionCommand}},     // 2 errors: unknown event + missing command
		{ID: "bad2", Event: EventPostToolUse, Action: Action{Type: "weird"}}, // 1 error: unknown action type
	}
	err := Validate(hooks)
	if err == nil {
		t.Fatal("expected aggregated errors")
	}
	msg := err.Error()
	for _, want := range []string{`unknown event "nope"`, "action.command must be non-empty", `unknown action.type "weird"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("aggregated error missing %q, got: %s", want, msg)
		}
	}
}

func TestValidateAcceptsGoodConfig(t *testing.T) {
	hooks := []Hook{
		{ID: "fmt", Event: EventPostToolUse, Action: Action{Type: ActionCommand, Command: "echo ok"}},
		{ID: "ctx", Event: EventSessionStart, Action: Action{Type: ActionPrompt, Message: "hello"}},
		{ID: "slack", Event: EventPostToolUse, Action: Action{Type: ActionHTTP, URL: "https://hooks.slack.com/services/xxx"}},
		{ID: "review", Event: EventPostToolUse, Action: Action{Type: ActionAgent, Message: "review the change"}},
	}
	if err := Validate(hooks); err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	h := Hook{
		ID: "slow",
		Action: Action{
			Type:    ActionCommand,
			Command: "sleep 2",
			Timeout: 100 * time.Millisecond,
		},
	}
	start := time.Now()
	result := runCommand(h, HookContext{EventName: EventPostToolUse, ToolName: "Bash"})
	elapsed := time.Since(start)

	if result.Success {
		t.Fatalf("expected timed-out command to report Success=false, output=%q", result.Output)
	}
	if !strings.Contains(result.Output, "timed out") {
		t.Fatalf("expected output to mention timeout, got: %q", result.Output)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("expected command to be killed near 100ms, but took %s", elapsed)
	}
}

func TestRunCommandDefaultTimeoutAllowsFastCommand(t *testing.T) {
	// Timeout=0 should fall back to defaultHookTimeout (10min) and not
	// strangle a sub-second command.
	h := Hook{
		ID: "fast",
		Action: Action{
			Type:    ActionCommand,
			Command: "echo ok",
		},
	}
	result := runCommand(h, HookContext{EventName: EventPostToolUse, ToolName: "Bash"})
	if !result.Success {
		t.Fatalf("expected fast command to succeed under default timeout, got output=%q", result.Output)
	}
	if !strings.Contains(result.Output, "ok") {
		t.Fatalf("expected output to contain stdout 'ok', got %q", result.Output)
	}
}
