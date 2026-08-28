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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanMemoryFilesEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := ScanMemoryFiles(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 headers, got %d", len(got))
	}
}

func TestScanMemoryFilesNonexistentDir(t *testing.T) {
	got, _ := ScanMemoryFiles(context.Background(), "/nonexistent/path/xyzzy", "")
	if len(got) != 0 {
		t.Errorf("expected empty result for missing dir, got %d", len(got))
	}
}

func TestScanMemoryFilesSortAndFilter(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	writeMD(t, filepath.Join(dir, "MEMORY.md"), "should be excluded")
	writeMDWithMtime(t, filepath.Join(dir, "old.md"), `---
name: old
description: older one
type: user
---
body`, now.Add(-48*time.Hour))
	writeMDWithMtime(t, filepath.Join(dir, "no_fm.md"), "body without frontmatter", now.Add(-12*time.Hour))
	writeMDWithMtime(t, filepath.Join(dir, "new.md"), `---
name: new
description: newer one
type: project
---
body`, now)
	writeMD(t, filepath.Join(dir, "skip.txt"), "not a markdown")

	got, err := ScanMemoryFiles(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 headers (MEMORY.md and .txt excluded), got %d: %+v", len(got), got)
	}
	if got[0].Filename != "new.md" {
		t.Errorf("expected newest-first ordering; got[0] = %q", got[0].Filename)
	}
	if got[0].Type != TypeProject || got[0].Description != "newer one" {
		t.Errorf("frontmatter parsed wrong on new.md: %+v", got[0])
	}
}

func TestScanMemoryFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub", "deeper")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMD(t, filepath.Join(sub, "nested.md"), "---\ntype: user\n---\nbody")

	got, _ := ScanMemoryFiles(context.Background(), dir, "")
	if len(got) != 1 {
		t.Fatalf("expected 1 nested header, got %d", len(got))
	}
	if got[0].Filename != filepath.Join("sub", "deeper", "nested.md") {
		t.Errorf("filename should be relative to memoryDir, got %q", got[0].Filename)
	}
}

func TestScanMemoryFilesCap(t *testing.T) {
	dir := t.TempDir()
	for i := range MaxMemoryFiles + 50 {
		writeMD(t, filepath.Join(dir, fmt.Sprintf("m%03d.md", i)), "---\ntype: user\n---\nbody")
	}
	got, _ := ScanMemoryFiles(context.Background(), dir, "")
	if len(got) != MaxMemoryFiles {
		t.Errorf("expected cap %d, got %d", MaxMemoryFiles, len(got))
	}
}

func TestFormatMemoryManifest(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := FormatMemoryManifest(nil); got != "" {
			t.Errorf("empty input should give empty string, got %q", got)
		}
	})

	t.Run("with type and description", func(t *testing.T) {
		got := FormatMemoryManifest([]MemoryHeader{{
			Filename:    "foo.md",
			MtimeMs:     1700000000000,
			Description: "a user note",
			Type:        TypeUser,
		}})
		if !strings.Contains(got, "[user]") {
			t.Errorf("missing type tag: %s", got)
		}
		if !strings.Contains(got, "foo.md") {
			t.Errorf("missing filename: %s", got)
		}
		if !strings.HasSuffix(got, ": a user note") {
			t.Errorf("missing description suffix: %s", got)
		}
	})

	t.Run("without type", func(t *testing.T) {
		got := FormatMemoryManifest([]MemoryHeader{{
			Filename: "foo.md",
			MtimeMs:  1700000000000,
		}})
		if strings.Contains(got, "[") {
			t.Errorf("no type expected; got: %s", got)
		}
	})

	t.Run("without description", func(t *testing.T) {
		got := FormatMemoryManifest([]MemoryHeader{{
			Filename: "foo.md",
			MtimeMs:  1700000000000,
			Type:     TypeFeedback,
		}})
		if strings.Contains(got, "): ") {
			t.Errorf("no `): ` description separator expected; got: %s", got)
		}
	})
}

func TestScanMemoryFilesContextCancel(t *testing.T) {
	dir := t.TempDir()
	for i := range 10 {
		writeMD(t, filepath.Join(dir, fmt.Sprintf("m%d.md", i)), "---\ntype: user\n---\nbody")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := ScanMemoryFiles(ctx, dir, "")
	if err != nil {
		t.Errorf("cancellation should not produce error, got: %v", err)
	}
	_ = got // size depends on race with cancel; we just verify it does not panic
}

func writeMD(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeMDWithMtime(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	writeMD(t, path, content)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}
