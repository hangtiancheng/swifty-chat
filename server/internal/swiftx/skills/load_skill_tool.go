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

// LoadSkillTool is the on-demand activation entry point. It's registered
// into the main tool registry at startup with progressive-disclosure
// semantics: the model sees a `## Available Skills` listing of every
// skill's name + description in the system prompt, and calls LoadSkill
// with the chosen name. The full SOP body is returned as the tool result
// so it enters the conversation as a regular message.
type LoadSkillTool struct {
	Catalog *Catalog
	Host    SkillHost
	// ForkHost provides the ability to run an isolated sub-agent; skills declaring
	// mode: fork depend on it. When nil (the host has not wired up a sub-agent
	// runtime) it falls back to inline, so the tool works on any host.
	ForkHost SkillForkHost
}

func (t *LoadSkillTool) Name() string { return "LoadSkill" }

func (t *LoadSkillTool) Category() tools.ToolCategory { return tools.CategoryRead }

func (t *LoadSkillTool) Description() string {
	return "Activate a Skill by name. Returns the full SOP body so you can follow its " +
		"instructions. Call this when the user's request matches one of the available " +
		"Skills listed in the available-skills section. Pass the Skill name without a leading slash."
}

func (t *LoadSkillTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The Skill name to activate (e.g. \"commit\", \"backend-interview\").",
				},
			},
			"required": []string{"name"},
		},
	}
}

func (t *LoadSkillTool) Execute(ctx context.Context, args map[string]any) tools.ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		return tools.ToolResult{Output: "name is required", IsError: true}
	}
	if t.Catalog == nil || t.Host == nil {
		return tools.ToolResult{Output: "LoadSkill not wired (Catalog or Host nil)", IsError: true}
	}
	skill, err := t.Catalog.GetFull(name)
	if err != nil && skill == nil {
		return tools.ToolResult{Output: fmt.Sprintf("unknown skill: %s", name), IsError: true}
	}
	if skill.PromptBody == "" {
		return tools.ToolResult{Output: fmt.Sprintf("skill %q has empty body — cannot activate", name), IsError: true}
	}

	// Fork mode: the SOP body does not enter the main conversation; it is handed to an
	// isolated sub-agent to execute, and only the final result is brought back. This way
	// the model loading a skill itself and the user invoking a slash command follow the
	// same mode semantics, and the declared isolation intent holds on both paths.
	if skill.Meta.IsFork() && t.ForkHost != nil {
		result, err := RunFork(ctx, skill, "", t.ForkHost)
		if err != nil {
			return tools.ToolResult{
				Output:  fmt.Sprintf("skill %q fork execution failed: %v", name, err),
				IsError: true,
			}
		}
		return tools.ToolResult{Output: result}
	}

	t.Host.ActivateSkill(skill.Meta.Name, skill.PromptBody)

	header := fmt.Sprintf("# Skill: %s\n\n", skill.Meta.Name)
	return tools.ToolResult{
		Output: header + skill.PromptBody,
	}
}
