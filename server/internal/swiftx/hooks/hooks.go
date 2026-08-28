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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// defaultHookTimeout is the cap applied when a hook config doesn't set its own `timeout`.
const defaultHookTimeout = 10 * time.Minute

type EventName string

const (
	EventSessionStart EventName = "session_start"
	EventSessionEnd   EventName = "session_end"
	EventTurnStart    EventName = "turn_start"
	EventTurnEnd      EventName = "turn_end"
	EventPreSend      EventName = "pre_send"
	EventPostReceive  EventName = "post_receive"
	EventPreToolUse   EventName = "pre_tool_use"
	EventPostToolUse  EventName = "post_tool_use"
	EventShutdown     EventName = "shutdown"
)

type ActionType string

const (
	ActionCommand ActionType = "command"
	ActionPrompt  ActionType = "prompt"
	ActionHTTP    ActionType = "http"
	ActionAgent   ActionType = "agent"
)

type Action struct {
	Type    ActionType        `yaml:"type"`
	Command string            `yaml:"command"`
	Message string            `yaml:"message"`
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
	Timeout time.Duration     `yaml:"timeout"`
}

type Hook struct {
	ID        string    `yaml:"id"`
	Event     EventName `yaml:"event"`
	Condition string    `yaml:"if"`
	Action    Action    `yaml:"action"`
	Reject    bool      `yaml:"reject"`
	Once      bool      `yaml:"once"`
	Async     bool      `yaml:"async"`
	// OnError controls behaviour when the action fails.
	//   "fail"   — propagate the error (default for blocking hooks)
	//   "ignore" — log and continue
	//   "reject" — treat hook failure as a reject (pre_tool_use only)
	OnError string `yaml:"on_error"`
}

type HookContext struct {
	EventName EventName
	ToolName  string
	ToolArgs  map[string]any
	FilePath  string
	Message   string
	Error     string
}

type HookResult struct {
	HookID  string
	Output  string
	Success bool
	Reject  bool
}

type Engine struct {
	mu            sync.Mutex
	hooks         []Hook
	notifications []HookResult
	fired         map[string]bool // hook IDs that fired (for `once`)
	// AgentRunner executes agent-type hooks. Optional — when nil, agent hooks
	// return a clear "no runner registered" error rather than silently failing.
	AgentRunner func(prompt string, ctx HookContext) (string, error)
}

func NewEngine() *Engine {
	return &Engine{fired: make(map[string]bool)}
}

// validEventNames is the whitelist of event names accepted by Validate.
// Sourced from the EventName constants above so adding a new event is a
// single-line change there, not two.
var validEventNames = map[EventName]bool{
	EventSessionStart: true,
	EventSessionEnd:   true,
	EventTurnStart:    true,
	EventTurnEnd:      true,
	EventPreSend:      true,
	EventPostReceive:  true,
	EventPreToolUse:   true,
	EventPostToolUse:  true,
	EventShutdown:     true,
}

// Validate checks a slice of hooks for configuration mistakes that would
// otherwise surface as silent misbehaviour at run time. Each action type has
// its own required fields; URL hooks need a parseable http(s) URL; timeout
// must be non-negative.
//
// All errors are aggregated via errors.Join so a single call surfaces every
// problem at once instead of bailing on the first. Each error is prefixed
// with the hook id (or index when id is empty) and the offending field.
func Validate(hooks []Hook) error {
	var errs []error
	for i, h := range hooks {
		label := h.ID
		if label == "" {
			label = fmt.Sprintf("hook[%d]", i)
		} else {
			label = fmt.Sprintf("hook[%d] (id=%q)", i, h.ID)
		}

		if !validEventNames[h.Event] {
			errs = append(errs, fmt.Errorf("%s: unknown event %q", label, h.Event))
		}

		if h.Action.Timeout < 0 {
			errs = append(errs, fmt.Errorf("%s: action.timeout must be >= 0 (got %s)", label, h.Action.Timeout))
		}

		switch h.Action.Type {
		case ActionCommand:
			if strings.TrimSpace(h.Action.Command) == "" {
				errs = append(errs, fmt.Errorf("%s: action.command must be non-empty for type %q", label, h.Action.Type))
			}
		case ActionPrompt:
			if strings.TrimSpace(h.Action.Message) == "" {
				errs = append(errs, fmt.Errorf("%s: action.message must be non-empty for type %q", label, h.Action.Type))
			}
		case ActionHTTP:
			if strings.TrimSpace(h.Action.URL) == "" {
				errs = append(errs, fmt.Errorf("%s: action.url must be non-empty for type %q", label, h.Action.Type))
			} else {
				u, err := url.Parse(h.Action.URL)
				if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
					errs = append(errs, fmt.Errorf("%s: action.url must be a valid http(s) URL (got %q)", label, h.Action.URL))
				}
			}
		case ActionAgent:
			if strings.TrimSpace(h.Action.Message) == "" && strings.TrimSpace(h.Action.Command) == "" {
				errs = append(errs, fmt.Errorf("%s: action.message (or action.command as fallback) must be non-empty for type %q", label, h.Action.Type))
			}
		case "":
			errs = append(errs, fmt.Errorf("%s: action.type is required", label))
		default:
			errs = append(errs, fmt.Errorf("%s: unknown action.type %q (want command / prompt / http / agent)", label, h.Action.Type))
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) LoadHooks(hooks []Hook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.hooks = hooks
	e.fired = make(map[string]bool)
}

func (e *Engine) RunHooks(ctx HookContext) []HookResult {
	var results []HookResult
	for _, h := range e.snapshotHooks() {
		if h.Event != ctx.EventName {
			continue
		}
		if !e.shouldFire(h, ctx) {
			continue
		}
		if h.Async {
			go func(h Hook) {
				res := e.executeAction(h, ctx)
				e.recordNotification(res)
			}(h)
			results = append(results, HookResult{HookID: h.ID, Output: "(async)", Success: true})
			continue
		}
		result := e.executeAction(h, ctx)
		results = append(results, result)
		e.recordNotification(result)
	}
	return results
}

// RunPreToolHooks runs pre-tool-use hooks. Returns (rejected, message).
// Non-reject hooks still run for their side effects (notifications/HTTP/etc).
func (e *Engine) RunPreToolHooks(ctx HookContext) (bool, string) {
	for _, h := range e.snapshotHooks() {
		if h.Event != EventPreToolUse {
			continue
		}
		if !e.shouldFire(h, ctx) {
			continue
		}
		result := e.executeAction(h, ctx)
		e.recordNotification(result)
		// A hook may reject by config or by failing with on_error=reject.
		if h.Reject || (!result.Success && h.OnError == "reject") {
			msg := result.Output
			if msg == "" {
				msg = "blocked by hook " + h.ID
			}
			return true, msg
		}
	}
	return false, ""
}

func (e *Engine) shouldFire(h Hook, ctx HookContext) bool {
	if h.Condition != "" && !evaluateCondition(h.Condition, ctx) {
		return false
	}
	if h.Once {
		e.mu.Lock()
		defer e.mu.Unlock()
		if h.ID != "" && e.fired[h.ID] {
			return false
		}
		if h.ID != "" {
			e.fired[h.ID] = true
		}
	}
	return true
}

func (e *Engine) snapshotHooks() []Hook {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Hook, len(e.hooks))
	copy(out, e.hooks)
	return out
}

func (e *Engine) recordNotification(r HookResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.notifications = append(e.notifications, r)
}

func (e *Engine) DrainNotifications() []HookResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := e.notifications
	e.notifications = nil
	return n
}

// evaluateCondition supports:
//   - leaf: `var == "value"`, `var =~ /regex/`, `var =* "glob"`, `var != "value"`
//   - composite: `cond1 && cond2`, `cond1 || cond2`
//   - inverse: `!cond` (must precede the leaf, not split across operators)
//
// Parentheses are not supported; composition is strictly left-to-right with
// `&&` and `||` at the same precedence (consistent with simple shell style).
func evaluateCondition(condition string, ctx HookContext) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true
	}
	// Composite: walk the string, splitting on top-level && and ||.
	// We don't support quoting that contains those operators since hook
	// conditions are configured by the user — keep parsing dumb-and-obvious.
	if tokens := splitComposite(condition); len(tokens) > 1 {
		result := evaluateCondition(tokens[0].expr, ctx)
		for i := 1; i < len(tokens); i++ {
			rhs := evaluateCondition(tokens[i].expr, ctx)
			if tokens[i].op == "&&" {
				result = result && rhs
			} else {
				result = result || rhs
			}
		}
		return result
	}

	if strings.HasPrefix(condition, "!") {
		return !evaluateCondition(strings.TrimSpace(condition[1:]), ctx)
	}

	return evaluateLeaf(condition, ctx)
}

type compToken struct {
	op   string // "" for the first token, then "&&" or "||"
	expr string
}

func splitComposite(s string) []compToken {
	var out []compToken
	start := 0
	cur := ""
	for i := 0; i < len(s)-1; i++ {
		pair := s[i : i+2]
		if pair == "&&" || pair == "||" {
			out = append(out, compToken{op: cur, expr: strings.TrimSpace(s[start:i])})
			cur = pair
			start = i + 2
			i++
		}
	}
	out = append(out, compToken{op: cur, expr: strings.TrimSpace(s[start:])})
	if len(out) == 1 {
		// No splits found.
		return nil
	}
	return out
}

func evaluateLeaf(condition string, ctx HookContext) bool {
	for _, op := range []string{"!=", "=~", "=*", "=="} {
		if before, after, ok := strings.Cut(condition, op); ok {
			left := strings.TrimSpace(before)
			right := strings.Trim(strings.TrimSpace(after), `"'`)
			val := resolveVar(left, ctx)
			switch op {
			case "==":
				return val == right
			case "!=":
				return val != right
			case "=~":
				pattern := strings.Trim(right, "/")
				matched, _ := regexp.MatchString(pattern, val)
				return matched
			case "=*":
				matched, _ := filepath.Match(right, val)
				return matched
			}
		}
	}
	// No operator → treat as a truthy variable check.
	return resolveVar(condition, ctx) != ""
}

func resolveVar(name string, ctx HookContext) string {
	switch name {
	case "tool":
		return ctx.ToolName
	case "event":
		return string(ctx.EventName)
	case "file_path":
		return ctx.FilePath
	case "message":
		return ctx.Message
	}
	if after, ok := strings.CutPrefix(name, "args."); ok {
		key := after
		if ctx.ToolArgs != nil {
			if v, ok := ctx.ToolArgs[key]; ok {
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

func (e *Engine) executeAction(h Hook, ctx HookContext) HookResult {
	switch h.Action.Type {
	case ActionCommand:
		return runCommand(h, ctx)
	case ActionPrompt:
		return HookResult{
			HookID:  h.ID,
			Output:  h.Action.Message,
			Success: true,
			Reject:  h.Reject,
		}
	case ActionHTTP:
		return runHTTP(h, ctx)
	case ActionAgent:
		return e.runAgent(h, ctx)
	default:
		return HookResult{
			HookID:  h.ID,
			Output:  fmt.Sprintf("Unknown action type: %s", h.Action.Type),
			Success: false,
		}
	}
}

// runAgent invokes the optional AgentRunner with the hook's Message as a
// one-shot prompt. Returns a clear error when no runner is configured so
// users learn they need to wire up agent-type hooks in their main entry.
func (e *Engine) runAgent(h Hook, ctx HookContext) HookResult {
	if e.AgentRunner == nil {
		return HookResult{
			HookID:  h.ID,
			Output:  "agent-type hook configured but no AgentRunner registered",
			Success: false,
			Reject:  h.Reject,
		}
	}
	prompt := h.Action.Message
	if prompt == "" {
		prompt = h.Action.Command
	}
	output, err := e.AgentRunner(prompt, ctx)
	if err != nil {
		return HookResult{HookID: h.ID, Output: err.Error(), Success: false, Reject: h.Reject}
	}
	return HookResult{HookID: h.ID, Output: output, Success: true, Reject: h.Reject}
}

func runCommand(h Hook, ctx HookContext) HookResult {
	timeout := h.Action.Timeout
	if timeout <= 0 {
		timeout = defaultHookTimeout
	}
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", h.Action.Command)
	cmd.Env = append(cmd.Environ(),
		"SWIFTX_EVENT="+string(ctx.EventName),
		"SWIFTX_TOOL="+ctx.ToolName,
		"SWIFTX_FILE_PATH="+ctx.FilePath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}
	output = strings.TrimSpace(output)

	// Distinguish timeout from generic exec failure so users (and downstream
	// notifications) can tell why a hook bailed. CommandContext kills the
	// child via SIGKILL on deadline; the Run error itself is a vague
	// *exec.ExitError, so we check the context.
	if execCtx.Err() == context.DeadlineExceeded {
		msg := fmt.Sprintf("command timed out after %s", timeout)
		if output != "" {
			msg = msg + ": " + output
		}
		return HookResult{
			HookID:  h.ID,
			Output:  msg,
			Success: false,
			Reject:  h.Reject,
		}
	}
	return HookResult{
		HookID:  h.ID,
		Output:  output,
		Success: err == nil,
		Reject:  h.Reject,
	}
}

func runHTTP(h Hook, ctx HookContext) HookResult {
	method := strings.ToUpper(h.Action.Method)
	if method == "" {
		method = "POST"
	}
	timeout := h.Action.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	body := h.Action.Body
	if body == "" {
		payload := map[string]any{
			"event":     string(ctx.EventName),
			"tool":      ctx.ToolName,
			"tool_args": ctx.ToolArgs,
			"file_path": ctx.FilePath,
			"message":   ctx.Message,
			"error":     ctx.Error,
		}
		if b, err := json.Marshal(payload); err == nil {
			body = string(b)
		}
	}

	cctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, method, h.Action.URL, strings.NewReader(body))
	if err != nil {
		return HookResult{HookID: h.ID, Output: err.Error(), Success: false, Reject: h.Reject}
	}
	for k, v := range h.Action.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" && body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HookResult{HookID: h.ID, Output: err.Error(), Success: false, Reject: h.Reject}
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	return HookResult{
		HookID:  h.ID,
		Output:  fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes))),
		Success: ok,
		Reject:  h.Reject,
	}
}
