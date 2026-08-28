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

import "context"

type ExitPlanModeTool struct {
	IsPlanMode func() bool
	PlanExists func() bool
}

func (t *ExitPlanModeTool) Name() string { return "ExitPlanMode" }

func (t *ExitPlanModeTool) Description() string {
	return "Exit plan mode and present the plan for user approval. " +
		"Call this when your plan is complete and written to the plan file."
}

func (t *ExitPlanModeTool) Category() ToolCategory { return CategoryRead }

func (t *ExitPlanModeTool) Schema() map[string]any {
	return map[string]any{
		"name":        "ExitPlanMode",
		"description": t.Description(),
		"input_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *ExitPlanModeTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	if t.IsPlanMode != nil && !t.IsPlanMode() {
		return ToolResult{
			Output:  "You are not in plan mode. This tool is only for exiting plan mode after writing a plan.",
			IsError: true,
		}
	}
	if t.PlanExists != nil && !t.PlanExists() {
		return ToolResult{
			Output:  "No plan file found. Please write your plan to the plan file before calling ExitPlanMode.",
			IsError: true,
		}
	}
	return ToolResult{
		Output: "Plan mode will be exited after this turn. " +
			"The user will be shown the plan approval dialog. " +
			"Do not call any more tools — end your turn now.",
	}
}
