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
	"strings"
	"testing"
)

// isolateHome redirects the user-level memory directory to a temporary directory.
// os.UserHomeDir reads USERPROFILE on Windows and HOME on Unix-like systems; both
// must be set, otherwise tests would fall through to the real ~/.swiftx/memory and
// interfere with other parallel tests.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestGetAutoMemPath(t *testing.T) {
	t.Setenv("SWIFTX_REMOTE_MEMORY_DIR", "")
	projectRoot := filepath.Join(t.TempDir(), "project")
	os.MkdirAll(projectRoot, 0o755)
	path := GetAutoMemPath(projectRoot)
	wantSuffix := filepath.Join(".swiftx", "memory") + string(filepath.Separator)
	if !strings.HasSuffix(path, wantSuffix) {
		t.Errorf("expected suffix %q, got: %s", wantSuffix, path)
	}
	absRoot, _ := filepath.Abs(projectRoot)
	if !strings.HasPrefix(path, absRoot) {
		t.Errorf("expected path under project root %s, got: %s", absRoot, path)
	}
}

func TestGetAutoMemPathRespectsOverride(t *testing.T) {
	overrideDir := filepath.Join(t.TempDir(), "custom", "memdir")
	t.Setenv("SWIFTX_REMOTE_MEMORY_DIR", overrideDir)
	path := GetAutoMemPath(filepath.Join(t.TempDir(), "anything"))
	expected := overrideDir + string(filepath.Separator)
	if path != expected {
		t.Errorf("override not honored: got %q, want %q", path, expected)
	}
}

func TestIsAutoMemPath(t *testing.T) {
	t.Setenv("SWIFTX_REMOTE_MEMORY_DIR", "")
	root := "/tmp/p"
	dir := GetAutoMemPath(root)
	cases := map[string]bool{
		dir + "MEMORY.md":         true,
		dir + "foo.md":            true,
		dir + "sub/foo.md":        true,
		"/tmp/p/.swiftx/memory-x": false,
		"/other/path/foo.md":      false,
	}
	for path, want := range cases {
		if got := IsAutoMemPath(path, root); got != want {
			t.Errorf("IsAutoMemPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestParseMemoryType(t *testing.T) {
	cases := map[string]MemoryType{
		"user":      TypeUser,
		"feedback":  TypeFeedback,
		"project":   TypeProject,
		"reference": TypeReference,
	}
	for in, want := range cases {
		got, ok := ParseMemoryType(in)
		if !ok || got != want {
			t.Errorf("ParseMemoryType(%q) = (%q, %v); want (%q, true)", in, got, ok, want)
		}
	}
	if _, ok := ParseMemoryType("unknown"); ok {
		t.Errorf("ParseMemoryType(unknown) should return false")
	}
	if _, ok := ParseMemoryType(""); ok {
		t.Errorf("ParseMemoryType empty should return false")
	}
}

func TestTruncateEntrypointContent(t *testing.T) {
	t.Run("under limits", func(t *testing.T) {
		raw := "- one\n- two\n- three"
		got := TruncateEntrypointContent(raw)
		if got.WasLineTruncated || got.WasByteTruncated {
			t.Errorf("should not truncate small content; got %+v", got)
		}
		if got.Content != raw {
			t.Errorf("content modified: %q", got.Content)
		}
	})

	t.Run("line cap", func(t *testing.T) {
		var lines []string
		for range MaxEntrypointLines + 10 {
			lines = append(lines, "x")
		}
		raw := strings.Join(lines, "\n")
		got := TruncateEntrypointContent(raw)
		if !got.WasLineTruncated {
			t.Errorf("expected line truncation")
		}
		if !strings.Contains(got.Content, "WARNING") {
			t.Errorf("truncation warning missing")
		}
	})

	t.Run("byte cap", func(t *testing.T) {
		raw := strings.Repeat("xxxxxxxxxx", MaxEntrypointBytes/5) + "\nextra"
		got := TruncateEntrypointContent(raw)
		if !got.WasByteTruncated {
			t.Errorf("expected byte truncation")
		}
	})
}

func TestLoadAutoMemoryPrompt(t *testing.T) {
	t.Setenv("SWIFTX_REMOTE_MEMORY_DIR", "")
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	prompt := LoadAutoMemoryPrompt(root)
	for _, want := range []string{
		"# auto memory",
		"## Types of memory",
		"## What NOT to save in memory",
		"## How to save memories",
		"## When to access memories",
		"## Before recommending from memory",
		"User-level " + AutoMemEntrypointName,
		"Project-level " + AutoMemEntrypointName,
		"is currently empty",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q in auto memory prompt", want)
		}
	}
}

func TestManagerLoadAll(t *testing.T) {
	t.Setenv("SWIFTX_REMOTE_MEMORY_DIR", "")
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mgr := NewManager(root)
	dir := mgr.Dir()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "user_role.md"), `---
name: user-role
description: user is a Go engineer
type: user
---

Body content.
`)
	mustWriteFile(t, filepath.Join(dir, "MEMORY.md"), "- [user-role](user_role.md) — user is a Go engineer\n")
	mustWriteFile(t, filepath.Join(dir, "skip.txt"), "not a memory")

	files := mgr.LoadAll()
	if len(files) != 1 {
		t.Fatalf("expected 1 memory file (MEMORY.md and skip.txt excluded), got %d", len(files))
	}
	f := files[0]
	if f.Name != "user-role" || f.Type != TypeUser {
		t.Errorf("frontmatter parsed wrong: %+v", f)
	}

	got := mgr.GetMemories()
	if len(got) != 1 || !strings.Contains(got[0], "[user]") {
		t.Errorf("GetMemories returned %v", got)
	}
}

func TestManagerClear(t *testing.T) {
	t.Setenv("SWIFTX_REMOTE_MEMORY_DIR", "")
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mgr := NewManager(root)
	dir := mgr.Dir()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "a.md"), "---\nname: a\ntype: user\n---\n")
	mustWriteFile(t, filepath.Join(dir, "MEMORY.md"), "- [a](a.md)\n")

	mgr.Clear()

	if files := mgr.LoadAll(); len(files) != 0 {
		t.Errorf("expected 0 files after Clear, got %d", len(files))
	}
	if _, err := os.Stat(mgr.EntrypointPath()); !os.IsNotExist(err) {
		t.Errorf("MEMORY.md should be removed; stat err = %v", err)
	}
}

func TestBuildSystemReminderIncludesExistingIndex(t *testing.T) {
	t.Setenv("SWIFTX_REMOTE_MEMORY_DIR", "")
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mgr := NewManager(root)

	if err := os.MkdirAll(mgr.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	indexLine := "- [previous-memory](prev.md) — saved earlier"
	mustWriteFile(t, mgr.EntrypointPath(), indexLine+"\n")

	prompt := mgr.BuildSystemReminder()
	if !strings.Contains(prompt, indexLine) {
		t.Errorf("system reminder did not include MEMORY.md content:\n%s", prompt)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
