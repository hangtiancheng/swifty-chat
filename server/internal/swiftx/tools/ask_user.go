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
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Question struct {
	Text        string           `json:"question"`
	Header      string           `json:"header"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
}

type QuestionRequest struct {
	Questions []Question
}

type QuestionResponse struct {
	Answers map[string]string
}

type AskUserQuestionTool struct {
	RequestCh chan<- AskUserRequest
}

type AskUserRequest struct {
	Questions  []Question
	ResponseCh chan QuestionResponse
}

func (t *AskUserQuestionTool) Name() string { return "AskUserQuestion" }

func (t *AskUserQuestionTool) Description() string {
	return `Ask the user a question with structured multiple-choice options. Use this to:
- Gather user preferences or requirements
- Clarify ambiguous instructions
- Get decisions on implementation choices
- Offer choices about direction to take

Each question has 2-4 options. An "Other" option for custom input is automatically provided.
Use multiSelect: true when choices are not mutually exclusive.`
}

func (t *AskUserQuestionTool) Category() ToolCategory { return CategoryRead }

func (t *AskUserQuestionTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"questions": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"question": map[string]any{
								"type":        "string",
								"description": "The question to ask the user",
							},
							"header": map[string]any{
								"type":        "string",
								"description": "Short label (max 12 chars)",
							},
							"options": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"label":       map[string]any{"type": "string", "description": "Option display text (1-5 words)"},
										"description": map[string]any{"type": "string", "description": "What this option means"},
									},
									"required": []string{"label", "description"},
								},
								"minItems": 2,
								"maxItems": 4,
							},
							"multiSelect": map[string]any{
								"type":    "boolean",
								"default": false,
							},
						},
						"required": []string{"question", "header", "options", "multiSelect"},
					},
					"minItems": 1,
					"maxItems": 4,
				},
			},
			"required": []string{"questions"},
		},
	}
}

func (t *AskUserQuestionTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	questionsRaw, ok := args["questions"]
	if !ok {
		return ToolResult{Output: "Error: questions is required", IsError: true}
	}

	questionsJSON, err := json.Marshal(questionsRaw)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: invalid questions format: %s", err), IsError: true}
	}

	var questions []Question
	if err := json.Unmarshal(questionsJSON, &questions); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: invalid questions format: %s", err), IsError: true}
	}

	if len(questions) == 0 || len(questions) > 4 {
		return ToolResult{Output: "Error: must have 1-4 questions", IsError: true}
	}

	for _, q := range questions {
		if len(q.Options) < 2 || len(q.Options) > 4 {
			return ToolResult{Output: fmt.Sprintf("Error: question '%s' must have 2-4 options", q.Text), IsError: true}
		}
	}

	if t.RequestCh == nil {
		return ToolResult{Output: "Error: AskUserQuestion not available in this context", IsError: true}
	}

	respCh := make(chan QuestionResponse, 1)
	t.RequestCh <- AskUserRequest{
		Questions:  questions,
		ResponseCh: respCh,
	}

	select {
	case resp := <-respCh:
		var parts []string
		for q, a := range resp.Answers {
			parts = append(parts, fmt.Sprintf("%q = %q", q, a))
		}
		return ToolResult{
			Output: fmt.Sprintf("User has answered your questions: %s. You can now continue with the user's answers in mind.", strings.Join(parts, ", ")),
		}
	case <-ctx.Done():
		return ToolResult{Output: "Question cancelled", IsError: true}
	}
}
