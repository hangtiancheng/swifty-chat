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

package skills

import (
	"strings"
)

type SkillMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	WhenToUse   string   `yaml:"when_to_use"`
	Tags        []string `yaml:"tags"`
	// Mode selects the execution mode. "inline" (default) injects the skill body into the current
	// conversation; "fork" runs the skill body in a sub-agent with isolated context.
	Mode string `yaml:"mode"`
	// Model overrides the LLM used for this skill. Empty = inherit main loop.
	Model string `yaml:"model"`
	// Context is kept for backward compatibility with old skills that used `context: fork` to mean
	// Mode=fork. Treated as fork mode if value == "fork".
	Context string `yaml:"context"`
	// ForkContext controls how much of the parent conversation gets carried into the forked sub-agent.
	// Only meaningful when Mode == "fork". Values: "full" (LLM summary of parent), "recent" (last 5
	// messages), "none" (no parent context, default).
	ForkContext string `yaml:"fork_context"`
}

// IsFork reports whether the skill should run in fork mode. Checks both Mode and the legacy Context
// field for backward compatibility.
func (m SkillMeta) IsFork() bool {
	return m.Mode == "fork" || m.Context == "fork"
}

type Skill struct {
	Meta       SkillMeta
	PromptBody string
	SourceDir  string
	// IsDirectory marks skills whose SourceDir contains additional resources (references/, scripts/).
	// True for directory-type skills that have supporting files on disk alongside SKILL.md.
	// False only for embedded skills that have no real directory on disk to access at runtime.
	IsDirectory bool
	// BodyLoaded marks whether PromptBody has been read from disk. Phase-1 loading only reads
	// frontmatter; the body stays empty until GetFull triggers a read.
	BodyLoaded bool
}

// Render returns the skill body with $ARGUMENTS substituted. If the body has no $ARGUMENTS
// placeholder and args is non-empty, the args are appended in a "## User Request" section.
//
// The execution mode is determined by Meta.Mode and is not reflected in the rendered
// result: an inline skill's body goes straight into the main conversation, while a fork
// skill's body is handed by the caller to an isolated sub-agent. Both paths receive the
// same text rendered here.
func (s *Skill) Render(args string) string {
	body := s.PromptBody
	if strings.Contains(body, "$ARGUMENTS") {
		return strings.ReplaceAll(body, "$ARGUMENTS", args)
	}
	if strings.TrimSpace(args) == "" {
		return body
	}
	return body + "\n\n## User Request\n\n" + args
}
