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

package tui

import (
	"strings"
	"testing"
)

func TestRenderDiffLinesColorsByPrefix(t *testing.T) {
	input := "Updated foo.go with 1 addition and 1 removal\n" +
		"   10  unchanged\n" +
		"-   11  old line\n" +
		"+   11  new line"

	got := renderDiffLines(input)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 rendered lines, got %d: %q", len(lines), got)
	}

	if lines[0] != toolDetailStyle.Render("Updated foo.go with 1 addition and 1 removal") {
		t.Errorf("summary line should use toolDetailStyle, got %q", lines[0])
	}
	if lines[1] != toolDetailStyle.Render("   10  unchanged") {
		t.Errorf("context line should use toolDetailStyle, got %q", lines[1])
	}
	if lines[2] != diffRemoveStyle.Render("-   11  old line") {
		t.Errorf("removed line should use diffRemoveStyle, got %q", lines[2])
	}
	if lines[3] != diffAddStyle.Render("+   11  new line") {
		t.Errorf("added line should use diffAddStyle, got %q", lines[3])
	}
}

func TestAppendEditDiffOnlyForEditFile(t *testing.T) {
	var sb strings.Builder
	appendEditDiff(&sb, []toolBlockInfo{{toolName: "Bash", output: "+ should not colorize"}})
	if sb.Len() != 0 {
		t.Errorf("non-EditFile tool should not append diff body, got %q", sb.String())
	}

	sb.Reset()
	appendEditDiff(&sb, []toolBlockInfo{{toolName: "EditFile", output: "+    1  hello"}})
	if !strings.Contains(sb.String(), "hello") {
		t.Errorf("EditFile diff body should be appended, got %q", sb.String())
	}

	sb.Reset()
	appendEditDiff(&sb, []toolBlockInfo{{toolName: "EditFile", output: ""}})
	if sb.Len() != 0 {
		t.Errorf("empty output should append nothing, got %q", sb.String())
	}
}
