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
	"fmt"
	"strings"
)

const (
	diffContextLines = 3
	// Cap diff output to prevent very large files from producing excessive
	// diff text that would slow down TUI rendering and consume context.
	maxDiffLines = 200
)

// DiffResult is the return value of BuildDiff: diff text with line numbers
// plus counts of added and removed lines.
type DiffResult struct {
	Text      string
	Additions int
	Removals  int
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// BuildDiff compares the file content before and after an edit and produces
// a line-numbered diff. It exploits the fact that edits only change a small
// region in the middle: common prefix and suffix lines are found from both
// ends, avoiding a full LCS/Myers diff (faster for large files and simpler).
func BuildDiff(oldContent, newContent string) DiffResult {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	prefixLen := 0
	maxPrefix := min(len(oldLines), len(newLines))
	for prefixLen < maxPrefix && oldLines[prefixLen] == newLines[prefixLen] {
		prefixLen++
	}

	suffixLen := 0
	maxSuffix := maxPrefix - prefixLen
	for suffixLen < maxSuffix &&
		oldLines[len(oldLines)-1-suffixLen] == newLines[len(newLines)-1-suffixLen] {
		suffixLen++
	}

	removedLines := oldLines[prefixLen : len(oldLines)-suffixLen]
	addedLines := newLines[prefixLen : len(newLines)-suffixLen]

	contextStart := max(0, prefixLen-diffContextLines)
	contextBefore := oldLines[contextStart:prefixLen]
	contextEnd := min(len(oldLines), len(oldLines)-suffixLen+diffContextLines)
	contextAfter := oldLines[len(oldLines)-suffixLen : contextEnd]

	var out []string
	oldLineNo := contextStart + 1
	newLineNo := contextStart + 1
	truncated := false

	push := func(prefix string, lineNo int, content string) {
		if len(out) >= maxDiffLines {
			truncated = true
			return
		}
		out = append(out, fmt.Sprintf("%s %4d  %s", prefix, lineNo, content))
	}

	for _, l := range contextBefore {
		push(" ", oldLineNo, l)
		oldLineNo++
		newLineNo++
	}
	for _, l := range removedLines {
		push("-", oldLineNo, l)
		oldLineNo++
	}
	for _, l := range addedLines {
		push("+", newLineNo, l)
		newLineNo++
	}
	for _, l := range contextAfter {
		push(" ", oldLineNo, l)
		oldLineNo++
		newLineNo++
	}

	if truncated {
		out = append(out, fmt.Sprintf("  … (diff truncated at %d lines)", maxDiffLines))
	}

	return DiffResult{
		Text:      strings.Join(out, "\n"),
		Additions: len(addedLines),
		Removals:  len(removedLines),
	}
}
