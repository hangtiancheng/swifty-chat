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

// SyntheticOutputTool lets the Agent deliver its final result as structured
// data. In non-interactive and coordinator modes, callers expect directly
// parseable JSON rather than prose embedded in natural language.
type SyntheticOutputTool struct {
	// JSONSchema is optional. When set, the output is validated against the
	// structure agreed upon with the caller.
	JSONSchema map[string]any
}

func (t *SyntheticOutputTool) Name() string           { return "SyntheticOutput" }
func (t *SyntheticOutputTool) Category() ToolCategory { return CategoryRead }

func (t *SyntheticOutputTool) Description() string {
	return "Return structured output in JSON format. Use this tool to return your final response " +
		"as structured data in non-interactive or coordinator mode sessions."
}

func (t *SyntheticOutputTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"output": map[string]any{
					"description": "The structured result: an object, an array, or a plain string",
				},
			},
			"required": []string{"output"},
		},
	}
}

func (t *SyntheticOutputTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	output, ok := args["output"]
	if !ok {
		return ToolResult{Output: "Error: output is required", IsError: true}
	}

	if err := t.validateSchema(output); err != "" {
		return ToolResult{
			Output:  fmt.Sprintf("Output does not match required schema: %s", err),
			IsError: true,
		}
	}

	// Strings are returned as-is without a secondary JSON wrapping.
	if s, isString := output.(string); isString {
		return ToolResult{Output: s}
	}

	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return ToolResult{
			Output:  fmt.Sprintf("Error: output is not serializable: %s", err),
			IsError: true,
		}
	}
	return ToolResult{Output: string(encoded)}
}

// validateSchema covers only top-level type and required fields; an empty
// string return means validation passed. Full JSON Schema validation is
// unnecessary here — this guards against obviously malformed delivery
// structures from the model.
func (t *SyntheticOutputTool) validateSchema(data any) string {
	if t.JSONSchema == nil {
		return ""
	}

	if expected, ok := t.JSONSchema["type"].(string); ok {
		switch expected {
		case "object":
			if _, isMap := data.(map[string]any); !isMap {
				return fmt.Sprintf("Expected object, got %T", data)
			}
		case "array":
			if _, isSlice := data.([]any); !isSlice {
				return fmt.Sprintf("Expected array, got %T", data)
			}
		case "string":
			if _, isString := data.(string); !isString {
				return fmt.Sprintf("Expected string, got %T", data)
			}
		}
	}

	required, hasRequired := t.JSONSchema["required"].([]any)
	obj, isObj := data.(map[string]any)
	if hasRequired && isObj {
		var missing []string
		for _, key := range required {
			name, _ := key.(string)
			if name == "" {
				continue
			}
			if _, present := obj[name]; !present {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return "Missing required fields: " + strings.Join(missing, ", ")
		}
	}

	return ""
}
