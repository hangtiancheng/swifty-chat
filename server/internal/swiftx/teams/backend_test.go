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
	"runtime"
	"testing"
)

// detectBackendFromEnv only inspects environment variables and is
// platform-independent, so it can be verified on any system.
func TestDetectBackendFromEnv(t *testing.T) {
	t.Run("inside tmux picks tmux", func(t *testing.T) {
		t.Setenv("TMUX", "/tmp/sock,1,0")
		t.Setenv("ITERM_SESSION_ID", "")
		if got := detectBackendFromEnv(); got != ModeTmux {
			t.Errorf("got %q, want tmux", got)
		}
	})
	t.Run("inside iterm2 picks iterm", func(t *testing.T) {
		t.Setenv("TMUX", "")
		t.Setenv("ITERM_SESSION_ID", "w0t0p0:ABC")
		if got := detectBackendFromEnv(); got != ModeITerm {
			t.Errorf("got %q, want iterm", got)
		}
	})
	t.Run("tmux wins over iterm", func(t *testing.T) {
		t.Setenv("TMUX", "/tmp/sock,1,0")
		t.Setenv("ITERM_SESSION_ID", "w0t0p0:ABC")
		if got := detectBackendFromEnv(); got != ModeTmux {
			t.Errorf("got %q, want tmux", got)
		}
	})
	t.Run("plain terminal falls back to in-process", func(t *testing.T) {
		t.Setenv("TMUX", "")
		t.Setenv("ITERM_SESSION_ID", "")
		if got := detectBackendFromEnv(); got != ModeInProcess {
			t.Errorf("got %q, want in-process", got)
		}
	})
}

// On Windows, detectBackend must stay in-process (guardrail) regardless of
// whether it is inside a tmux session, to avoid pwsh spawn failures.
func TestDetectBackendWindowsGuard(t *testing.T) {
	t.Setenv("TMUX", "/tmp/sock,1,0")
	got := detectBackend()
	if runtime.GOOS == "windows" {
		if got != ModeInProcess {
			t.Errorf("windows must stay in-process even inside tmux, got %q", got)
		}
	} else {
		// On non-Windows platforms, being inside a tmux session should pick tmux.
		if got != ModeTmux {
			t.Errorf("non-windows inside tmux should pick tmux, got %q", got)
		}
	}
}
