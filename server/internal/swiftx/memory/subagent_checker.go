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

import "github.com/hangtiancheng/swifty-chat/server/internal/swiftx/permissions"

// NewSubAgentChecker builds the permission checker for the background memory
// agent (extraction and consolidation).
//
// The sandbox baseline uses the project root, consistent with the main agent.
// The user-level memory directory lives outside the project root, so it is
// explicitly added as an extra allowed path for the background agent to write
// user/feedback memories. The rule engine reads the three project rule files,
// so background execution is bound by the user-configured deny/ask rules too.
func NewSubAgentChecker(projectRoot, userMemoryDir string) *permissions.Checker {
	var extraRoots []string
	if userMemoryDir != "" {
		extraRoots = append(extraRoots, userMemoryDir)
	}
	return permissions.NewChecker(
		permissions.NewPathSandbox(projectRoot, extraRoots...),
		permissions.NewRuleEngine(projectRoot),
		permissions.ModeBypass,
	)
}
