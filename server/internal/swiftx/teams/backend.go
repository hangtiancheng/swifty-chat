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

package teams

import (
	"os"
	"runtime"
)

// detectBackend: use a pane backend only when already inside a tmux / iTerm2
// session; otherwise fall back to in-process. Detection relies on environment
// variables — tmux and iTerm2 automatically set TMUX / ITERM_SESSION_ID for
// processes inside a session, requiring no manual configuration.
// DetectBackend is exported for external packages: the Agent tool auto-creates
// a Team when the specified one does not exist, and needs to pick a backend
// based on the current environment.
func DetectBackend() TeamMode { return detectBackend() }

func detectBackend() TeamMode {
	// Windows guard: tmux pane spawn executes POSIX commands via pwsh which
	// fails, so always use in-process on Windows.
	if runtime.GOOS == "windows" {
		return ModeInProcess
	}
	return detectBackendFromEnv()
}

// detectBackendFromEnv determines the backend solely from environment variables,
// extracted for unit testing (independent of the host platform).
func detectBackendFromEnv() TeamMode {
	if os.Getenv("TMUX") != "" {
		return ModeTmux
	}
	if os.Getenv("ITERM_SESSION_ID") != "" {
		return ModeITerm
	}
	return ModeInProcess
}
