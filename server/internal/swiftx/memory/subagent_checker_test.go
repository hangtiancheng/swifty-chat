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

package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"
)

func TestNewSubAgentCheckerOpensUserMemoryDir(t *testing.T) {
	// The sandbox allows the system temp dir by default, so use a path under
	// home to verify it is this specific path being opened.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot resolve home directory")
	}
	projectRoot := t.TempDir()
	userMemoryDir := filepath.Join(home, ".swiftx", "memory")

	chk := NewSubAgentChecker(projectRoot, userMemoryDir)

	// The user-level memory directory is outside the project root; the
	// background agent needs to write there, so it should be allowed.
	if ok, reason := chk.Sandbox.Check(filepath.Join(userMemoryDir, "MEMORY.md")); !ok {
		t.Errorf("user-level memory directory should be allowed, but was blocked: %s", reason)
	}

	// Paths inside the project are allowed as usual.
	if ok, reason := chk.Sandbox.Check(filepath.Join(projectRoot, "a.txt")); !ok {
		t.Errorf("in-project path should be allowed, but was blocked: %s", reason)
	}

	// Unrelated directories under home are unaffected and remain blocked.
	unrelated := filepath.Join(home, "unrelated-dir-for-swiftx-test", "x.txt")
	if ok, _ := chk.Sandbox.Check(unrelated); ok {
		t.Error("unrelated out-of-project directory should not be allowed")
	}
}

func TestNewSubAgentCheckerCarriesProjectRules(t *testing.T) {
	projectRoot := t.TempDir()

	chk := NewSubAgentChecker(projectRoot, "")

	// The rule engine reads the three project rule files; background
	// execution is bound by the user configuration as well.
	if want := filepath.Join(projectRoot, ".swiftx", "permissions.yaml"); chk.RuleEngine.ProjectPath != want {
		t.Errorf("ProjectPath = %q, want %q", chk.RuleEngine.ProjectPath, want)
	}
	if want := filepath.Join(projectRoot, ".swiftx", "permissions.local.yaml"); chk.RuleEngine.LocalPath != want {
		t.Errorf("LocalPath = %q, want %q", chk.RuleEngine.LocalPath, want)
	}

	// The background agent has no one to answer to, so it must run in bypass mode.
	if chk.Mode != permissions.ModeBypass {
		t.Errorf("Mode = %q, want bypassPermissions", chk.Mode)
	}
}
