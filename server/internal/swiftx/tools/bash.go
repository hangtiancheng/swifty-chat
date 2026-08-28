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
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/sandbox"
)

// commandErrorThresholds defines exit code thresholds for special commands:
// an exit code is only considered an error when it >= threshold.
// For example, grep exit code 1 means "no match" (not an error); exit code 2 is a syntax/IO error.
var commandErrorThresholds = map[string]int{
	"grep": 2, // exit 1 = no matches
	"rg":   2, // ripgrep, same as grep
	"diff": 2, // exit 1 = files differ
	"find": 2, // exit 1 = partial success (e.g. permission denied)
	"test": 2, // exit 1 = condition is false
	"[":    2, // alias for test
}

// interpretExitCode determines whether an exit code represents an error
// based on command semantics. For pipelines, the last command's exit code
// is used (bash default behaviour).
func interpretExitCode(command string, exitCode int) bool {
	if exitCode == 0 {
		return false
	}
	base := extractLastCommand(command)
	if threshold, ok := commandErrorThresholds[base]; ok {
		return exitCode >= threshold
	}
	// Default: any non-zero exit code is an error.
	return true
}

// extractLastCommand extracts the base command name of the last segment of a
// pipeline from the full command string.
// For example, "cat file | grep foo" → "grep", "timeout 5 rg pattern" → "rg".
func extractLastCommand(command string) string {
	// Take the last segment of the pipeline.
	parts := strings.Split(command, "|")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return ""
	}
	// Take the first token as the command, then strip any path prefix.
	fields := strings.Fields(last)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// exitCodeHint provides a semantic hint for non-error exit codes of well-known commands.
func exitCodeHint(command string, exitCode int) string {
	base := extractLastCommand(command)
	switch base {
	case "grep", "rg":
		if exitCode == 1 {
			return "no matches found"
		}
	case "diff":
		if exitCode == 1 {
			return "files differ"
		}
	case "find":
		if exitCode == 1 {
			return "some directories were inaccessible"
		}
	case "test", "[":
		if exitCode == 1 {
			return "condition is false"
		}
	}
	return fmt.Sprintf("command failed with exit code %d", exitCode)
}

const maxTimeout = 600

type BashTool struct {
	WorkDir       string
	Sandbox       sandbox.Sandbox // OS-level sandbox instance; nil means disabled.
	SandboxConfig sandbox.Config  // Sandbox path and network permission configuration.
}

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string { return BashDescription }

func (t *BashTool) Category() ToolCategory { return CategoryCommand }

// IsConcurrencySafe allows read-only commands to run concurrently with other
// read-only tools; mutating commands must run exclusively.
//
// Commands like ls, cat, and git status mutate no external state, just like
// ReadFile, so there is no risk of interference. Commands like rm, mv, and
// npm install would break the model-intended execution order if run concurrently.
// The check reuses the safe-command allowlist; redirections, pipes, command
// chaining, and command substitution are all excluded.
func (t *BashTool) IsConcurrencySafe(args map[string]any) bool {
	cmd, ok := args["command"].(string)
	return ok && IsSafeCommand(cmd)
}

func (t *BashTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to execute"},
				"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (max 600)", "default": 120},
			},
			"required": []string{"command"},
		},
	}
}

func (t *BashTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	command, _ := args["command"].(string)
	if command == "" {
		return ToolResult{Output: "Error: command is required", IsError: true}
	}

	timeout := min(intArg(args, "timeout", 120), maxTimeout)

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// If the sandbox is available, wrap the command for execution inside it.
	actualCommand := command
	if t.Sandbox != nil && t.Sandbox.Available() {
		wrapped, err := t.Sandbox.Wrap(command, t.SandboxConfig)
		if err == nil {
			actualCommand = wrapped
		}
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", actualCommand)
	// Merge stdout and stderr into a single stream to simplify output parsing.
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}

	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return ToolResult{Output: fmt.Sprintf("Error: command timed out after %ds", timeout), IsError: true}
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == nil {
			return ToolResult{Output: fmt.Sprintf("Error executing command: %s", err), IsError: true}
		}
	}

	var sb bytes.Buffer
	fmt.Fprintf(&sb, "$ %s\n", command)
	if combined.Len() > 0 {
		sb.Write(combined.Bytes())
		if combined.Bytes()[combined.Len()-1] != '\n' {
			sb.WriteByte('\n')
		}
	}
	if exitCode != 0 {
		fmt.Fprintf(&sb, "Exit code %d", exitCode)
		if !interpretExitCode(command, exitCode) {
			fmt.Fprintf(&sb, " (%s)", exitCodeHint(command, exitCode))
		}
	}

	// is_error is only true on timeout/interrupt; normal non-zero exit codes are not flagged as errors.
	return ToolResult{Output: sb.String(), IsError: false}
}
