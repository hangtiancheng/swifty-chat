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

package main

import (
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/hooks"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/remote"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/try_cache"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tui"
)

func main() {
	recordExit := try_cache.TryCatch()
	defer recordExit()
	// Persist the main goroutine's panic to disk before re-panicking; terminal output behavior is unchanged.
	defer func() {
		if r := recover(); r != nil {
			try_cache.RecordPanic("main", r, debug.Stack())
			panic(r)
		}
	}()

	if args, ok := parseTeammateFlags(os.Args[1:]); ok {
		if err := runTeammate(args); err != nil {
			fmt.Fprintf(os.Stderr, "teammate: %s\n", err)
			os.Exit(1)
		}
		return
	}

	// Parse -p/--print and --remote mode flags
	remoteAddr := ""
	var filteredArgs []string
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--remote" {
			remoteAddr = ":18888"
			if i+1 < len(os.Args) && os.Args[i+1][0] != '-' {
				remoteAddr = os.Args[i+1]
				i++
			}
		} else {
			filteredArgs = append(filteredArgs, os.Args[i])
		}
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	validHooks := cfg.Hooks
	if err := hooks.Validate(validHooks); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: hook configuration is invalid, starting with no hooks:\n%s\n", err)
		validHooks = nil
	}

	// -p/--print mode: non-interactive execution, outputs result and exits
	if userPrompt, outputFormat, ok := parsePrintFlags(os.Args[1:]); ok {
		if userPrompt == "" {
			// Read prompt from stdin
			buf, _ := os.ReadFile("/dev/stdin")
			userPrompt = string(buf)
		}
		if userPrompt == "" {
			fmt.Fprintf(os.Stderr, "Error: -p requires a prompt argument or stdin input\n")
			os.Exit(1)
		}
		if err := runPrint(userPrompt, cfg, validHooks, outputFormat); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	if remoteAddr != "" {
		srv := remote.NewServer(cfg.Providers, cfg.MCPServers, validHooks, remoteAddr, cfg.EnableCoordinatorMode, !cfg.ForkEnabled())
		if err := srv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Remote server error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	m := tui.New(cfg.Providers, cfg.MCPServers, validHooks, cfg.Sandbox)
	m.EnableCoordinatorMode = cfg.EnableCoordinatorMode
	m.ForkDisabled = !cfg.ForkEnabled()
	m.PermissionMode = cfg.PermissionMode

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
