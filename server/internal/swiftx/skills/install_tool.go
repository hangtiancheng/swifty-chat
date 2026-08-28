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
	"context"
	"fmt"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// InstallSkillTool lets the model install a new Skill on demand from a
// skills.sh / github.com URL the user provides.
//
// The OnInstalled callback is fired after a successful install with the
// new skill's name; the TUI uses it to re-register the slash command so
// `/<new-skill>` works without a restart.
type InstallSkillTool struct {
	Catalog     *Catalog
	OnInstalled func(name string)
	// InstallRoot overrides ~/.swiftx/skills for tests. Empty = derive
	// from UserSkillsRoot at call time.
	InstallRoot string
}

func (t *InstallSkillTool) Name() string { return "InstallSkill" }

func (t *InstallSkillTool) Category() tools.ToolCategory { return tools.CategoryWrite }

func (t *InstallSkillTool) Description() string {
	return "Download and install a Skill from a URL into the user-global skills directory " +
		"(~/.swiftx/skills/). Supports skills.sh URLs (https://www.skills.sh/<owner>/<repo>/<name>), " +
		"GitHub tree URLs (https://github.com/<owner>/<repo>/tree/<ref>/<path>), and raw " +
		"SKILL.md URLs. After install the Skill becomes available via /<name> and LoadSkill. " +
		"Call this when the user pastes a Skill URL and asks to install it."
}

func (t *InstallSkillTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The Skill URL to fetch. Examples: \"https://www.skills.sh/anthropics/skills/frontend-design\", \"https://github.com/anthropics/skills/tree/main/skills/pdf\".",
				},
			},
			"required": []string{"url"},
		},
	}
}

func (t *InstallSkillTool) Execute(_ context.Context, args map[string]any) tools.ToolResult {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return tools.ToolResult{Output: "url is required", IsError: true}
	}
	src, err := ParseSkillURL(rawURL)
	if err != nil {
		return tools.ToolResult{Output: err.Error(), IsError: true}
	}

	root := t.InstallRoot
	if root == "" {
		r, err := UserSkillsRoot()
		if err != nil {
			return tools.ToolResult{Output: err.Error(), IsError: true}
		}
		root = r
	}

	report, err := Install(src, root)
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("install failed: %v", err), IsError: true}
	}

	// Refresh the catalog so the new skill's frontmatter is indexed and
	// reachable via LoadSkill without a restart.
	if t.Catalog != nil {
		t.Catalog.Reload(t.Catalog.workDir)
	}
	if t.OnInstalled != nil {
		t.OnInstalled(report.SkillName)
	}

	return tools.ToolResult{
		Output: fmt.Sprintf(
			"Installed skill %q from %s into %s (%d files, %d bytes). Now available — call LoadSkill({name: %q}) or invoke /%s directly.",
			report.SkillName, src.Original, report.TargetDir, report.FileCount, report.TotalBytes, report.SkillName, report.SkillName,
		),
	}
}
