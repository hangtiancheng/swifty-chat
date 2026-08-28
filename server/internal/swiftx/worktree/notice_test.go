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

package worktree

import (
	"strings"
	"testing"
)

func TestBuildWorktreeNotice(t *testing.T) {
	notice := BuildWorktreeNotice("/home/user/project", "/home/user/project/.swiftx/worktrees/agent-a1234567")

	// Must contain both paths
	if !strings.Contains(notice, "/home/user/project") {

		t.Fatal("notice should contain parent CWD")
	}
	if !strings.Contains(notice, "agent-a1234567") {
		t.Fatal("notice should contain worktree path")
	}
	// Must mention isolation concepts

	if !strings.Contains(notice, "isolated") {
		t.Fatal("notice should mention isolation")
	}
	if !strings.Contains(notice, "worktree") {
		t.Fatal("notice should mention worktree")
	}
	if !strings.Contains(notice, "Re-read") {
		t.Fatal("notice should tell agent to re-read files")
	}
}
