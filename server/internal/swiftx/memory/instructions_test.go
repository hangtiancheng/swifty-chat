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

func TestLoadInstructionsBasic(t *testing.T) {
	dir := t.TempDir()
	mustInitGit(t, dir)

	mustWrite(t, filepath.Join(dir, "SWIFTX.md"), "root swiftx rules")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "root agents rules")
	mustWrite(t, filepath.Join(dir, ".swiftx", "SWIFTX.md"), "dotdir instructions")

	out := LoadInstructions(dir)
	for _, want := range []string{"root swiftx rules", "root agents rules", "dotdir instructions"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestLoadInstructionsWalksUp(t *testing.T) {
	root := t.TempDir()
	mustInitGit(t, root)
	sub := filepath.Join(root, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "SWIFTX.md"), "from root")
	mustWrite(t, filepath.Join(sub, "SWIFTX.md"), "from leaf")

	out := LoadInstructions(sub)
	rootIdx := strings.Index(out, "from root")
	leafIdx := strings.Index(out, "from leaf")
	if rootIdx == -1 || leafIdx == -1 {
		t.Fatalf("both files should appear; got:\n%s", out)
	}
	if rootIdx >= leafIdx {
		t.Errorf("leaf file should be ordered after root (higher priority); root=%d leaf=%d", rootIdx, leafIdx)
	}
}

// .swiftx/SWIFTX.md participates in the upward directory walk just like the
// root SWIFTX.md; the deeper directory's copy is ordered after the shallower
// one and thus has higher priority.
func TestLoadInstructionsDotDirWalksUp(t *testing.T) {
	root := t.TempDir()
	mustInitGit(t, root)
	sub := filepath.Join(root, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".swiftx", "SWIFTX.md"), "dotdir from root")
	mustWrite(t, filepath.Join(sub, ".swiftx", "SWIFTX.md"), "dotdir from leaf")

	out := LoadInstructions(sub)
	rootIdx := strings.Index(out, "dotdir from root")
	leafIdx := strings.Index(out, "dotdir from leaf")
	if rootIdx == -1 || leafIdx == -1 {
		t.Fatalf("both .swiftx files should appear; got:\n%s", out)
	}
	if rootIdx >= leafIdx {
		t.Errorf("leaf .swiftx file should be ordered after root; root=%d leaf=%d", rootIdx, leafIdx)
	}
}

// Within the same directory, SWIFTX.md is loaded before .swiftx/SWIFTX.md;
// the latter has higher priority.
func TestLoadInstructionsDotDirAfterPlain(t *testing.T) {
	dir := t.TempDir()
	mustInitGit(t, dir)
	mustWrite(t, filepath.Join(dir, "SWIFTX.md"), "plain file")
	mustWrite(t, filepath.Join(dir, ".swiftx", "SWIFTX.md"), "dotdir file")

	out := LoadInstructions(dir)
	plainIdx := strings.Index(out, "plain file")
	dotIdx := strings.Index(out, "dotdir file")
	if plainIdx == -1 || dotIdx == -1 {
		t.Fatalf("both files should appear; got:\n%s", out)
	}
	if plainIdx >= dotIdx {
		t.Errorf(".swiftx file should be ordered after the plain one; plain=%d dot=%d", plainIdx, dotIdx)
	}
}

func TestExpandIncludesResolvesRelative(t *testing.T) {
	dir := t.TempDir()
	mustInitGit(t, dir)

	mustWrite(t, filepath.Join(dir, "rules", "style.md"), "style rule")
	mustWrite(t, filepath.Join(dir, "SWIFTX.md"), "header\n@./rules/style.md\nfooter\n")

	out := LoadInstructions(dir)
	if !strings.Contains(out, "style rule") {
		t.Errorf("@include did not expand:\n%s", out)
	}
	if !strings.Contains(out, "header") || !strings.Contains(out, "footer") {
		t.Errorf("surrounding text was dropped:\n%s", out)
	}
}

func TestExpandIncludesIgnoresCycles(t *testing.T) {
	dir := t.TempDir()
	mustInitGit(t, dir)

	mustWrite(t, filepath.Join(dir, "a.md"), "from a\n@./b.md\n")
	mustWrite(t, filepath.Join(dir, "b.md"), "from b\n@./a.md\n")
	mustWrite(t, filepath.Join(dir, "SWIFTX.md"), "@./a.md\n")

	out := LoadInstructions(dir)
	if strings.Count(out, "from a") != 1 || strings.Count(out, "from b") != 1 {
		t.Errorf("cycle protection failed:\n%s", out)
	}
}

func TestExpandIncludesSkipsInsideCodeFences(t *testing.T) {
	dir := t.TempDir()
	mustInitGit(t, dir)

	mustWrite(t, filepath.Join(dir, "skipped.md"), "should not appear")
	mustWrite(t, filepath.Join(dir, "SWIFTX.md"), "```\n@./skipped.md\n```\n")

	out := LoadInstructions(dir)
	if strings.Contains(out, "should not appear") {
		t.Errorf("include inside fenced code block was expanded:\n%s", out)
	}
}

func TestParseIncludeOnlyAcceptsPathLike(t *testing.T) {
	cases := map[string]string{
		"@./foo.md":     "./foo.md",
		"@~/bar.md":     "~/bar.md",
		"@/abs/path.md": "/abs/path.md",
		"@../up.md":     "../up.md",
		"@username":     "",
		"@@escaped":     "",
		"@ ./space.md":  "",
		"plain text":    "",
	}
	for in, want := range cases {
		if got := parseInclude(in); got != want {
			t.Errorf("parseInclude(%q) = %q, want %q", in, got, want)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustInitGit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}
