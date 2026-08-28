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

import "fmt"

// BuildWorktreeNotice returns the notice text injected into sub-agent prompts when they run in an
// isolated worktree. Tells the child to translate paths from the inherited context and re-read
// files.
func BuildWorktreeNotice(parentCwd, worktreeCwd string) string {
	return fmt.Sprintf(
		"You've inherited the conversation context above from a parent agent working in %s. "+
			"You are operating in an isolated git worktree at %s — same repository, same relative "+

			"file structure, separate working copy. Paths in the inherited context refer to the "+
			"parent's working directory; translate them to your worktree root. Re-read files before "+
			"editing if the parent may have modified them since they appear in the context. Your "+
			"changes stay in this worktree and will not affect the parent's files.",

		parentCwd, worktreeCwd,
	)
}
