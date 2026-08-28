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

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

const openaiStreamIdleTimeout = 5 * time.Minute

type openaiClient struct {
	client          openai.Client
	model           string
	thinking        bool
	systemPrompt    string
	maxOutputTokens int
	contextWindow   int
}

func newOpenAIClient(cfg *config.ProviderConfig, systemPrompt string) (*openaiClient, error) {
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		return nil, &AuthenticationError{
			Message: "OpenAI API key not found. Set it in .swiftx/config.yaml or via OPENAI_API_KEY env var.",
		}
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(cfg.BaseURL),
	)

	return &openaiClient{
		client:          client,
		model:           cfg.Model,
		thinking:        cfg.Thinking,
		systemPrompt:    systemPrompt,
		maxOutputTokens: cfg.GetMaxOutputTokens(),
		contextWindow:   cfg.GetContextWindow(),
	}, nil
}

func (c *openaiClient) SetSystemPrompt(prompt string) {
	c.systemPrompt = prompt
}

func (c *openaiClient) SetMaxOutputTokens(tokens int) {
	c.maxOutputTokens = tokens
}

func (c *openaiClient) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent, 64)
	errs := make(chan error, 1)

	// Ensure tool call/result pairing before sending the request (same rationale as the Anthropic branch).
	input := buildOpenAIInput(conversation.EnsureToolPairing(conv.GetMessages()))

	var tools []responses.ToolUnionParam
	for _, s := range toolSchemas {
		name, _ := s["name"].(string)
		desc, _ := s["description"].(string)
		params, _ := s["parameters"].(map[string]any)
		tools = append(tools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        name,
				Description: param.NewOpt(desc),
				Parameters:  params,
				Strict:      param.NewOpt(false),
			},
		})
	}

	go func() {
		defer close(events)
		defer close(errs)

		reqParams := responses.ResponseNewParams{
			Model:        c.model,
			Instructions: param.NewOpt(c.systemPrompt),
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: input,
			},
		}
		if c.thinking {
			reqParams.Reasoning = shared.ReasoningParam{
				Effort:  shared.ReasoningEffortHigh,
				Summary: shared.ReasoningSummaryDetailed,
			}
			reqParams.Include = []responses.ResponseIncludable{
				responses.ResponseIncludableReasoningEncryptedContent,
			}
		}
		if len(tools) > 0 {
			reqParams.Tools = tools
		}

		stream := c.client.Responses.NewStreaming(ctx, reqParams)
		defer stream.Close()

		var currentToolName, currentCallID, jsonAccum string
		var reasoningID, reasoningText string

		// Read SSE events in a separate goroutine so we can respect ctx cancellation
		// and detect silent connection drops. The SDK's stream.Next() may block
		// indefinitely if the underlying connection dies without FIN/RST.
		type sseResult struct {
			hasNext bool
		}
		nextCh := make(chan sseResult, 1)

		readNext := func() {
			nextCh <- sseResult{hasNext: stream.Next()}
		}

		idle := time.NewTimer(openaiStreamIdleTimeout)
		defer idle.Stop()

		go readNext()
		for {
			var res sseResult
			select {
			case <-ctx.Done():
				errs <- &NetworkError{Message: fmt.Sprintf("context cancelled: %v", ctx.Err())}
				return
			case <-idle.C:
				errs <- &NetworkError{Message: fmt.Sprintf("stream idle timeout: no SSE events for %s", openaiStreamIdleTimeout)}
				return
			case res = <-nextCh:
			}

			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(openaiStreamIdleTimeout)

			if !res.hasNext {
				break
			}

			event := stream.Current()
			switch event.Type {
			case "response.output_text.delta":
				events <- TextDelta{Text: event.Delta.OfString}
			case "response.output_item.added":
				switch event.Item.Type {
				case "function_call":
					currentToolName = event.Item.Name
					currentCallID = event.Item.CallID
					jsonAccum = ""
					events <- ToolCallStart{ToolName: currentToolName, ToolID: currentCallID}
				case "reasoning":
					reasoningID = event.Item.ID
					reasoningText = ""
				}
			case "response.reasoning_summary_text.delta":
				reasoningText += event.Delta.OfString
				events <- ThinkingDelta{Text: event.Delta.OfString}
			case "response.reasoning_summary_text.done":
				events <- ThinkingComplete{Thinking: reasoningText, Signature: reasoningID}
			case "response.function_call_arguments.delta":
				jsonAccum += event.Delta.OfString
				events <- ToolCallDelta{Text: event.Delta.OfString}
			case "response.function_call_arguments.done":
				var args map[string]any
				if jsonAccum != "" {
					json.Unmarshal([]byte(jsonAccum), &args)
				}
				if args == nil {
					args = map[string]any{}
				}
				events <- ToolCallComplete{
					ToolID:    currentCallID,
					ToolName:  currentToolName,
					Arguments: args,
				}
				currentToolName = ""
				currentCallID = ""
				jsonAccum = ""
			case "response.completed":
				usage := UsageInfo{}
				if event.Response.Usage.InputTokens != 0 || event.Response.Usage.OutputTokens != 0 {
					usage.InputTokens = int(event.Response.Usage.InputTokens)
					usage.OutputTokens = int(event.Response.Usage.OutputTokens)
					// cache_read from input_tokens_details.cached_tokens; the
					// Responses API has no cache_creation counterpart, so it's 0.
					usage.CacheReadTokens = int(event.Response.Usage.InputTokensDetails.CachedTokens)
					// input_tokens already includes the cached prefix; subtract so the
					// usage anchor (input + cache_read) doesn't double-count it.
					usage.InputTokens -= usage.CacheReadTokens
					if usage.InputTokens < 0 {
						usage.InputTokens = 0
					}
				}
				events <- StreamEnd{StopReason: "end_turn", Usage: usage}
			}

			go readNext()
		}

		if err := stream.Err(); err != nil {
			errs <- classifyOpenAIError(err)
		}
	}()

	return events, errs
}

func buildOpenAIInput(messages []conversation.Message) responses.ResponseInputParam {
	var input responses.ResponseInputParam
	for _, m := range messages {
		if m.Role == "assistant" {
			for _, tb := range m.ThinkingBlocks {
				input = append(input, responses.ResponseInputItemParamOfReasoning(
					tb.Signature,
					[]responses.ResponseReasoningItemSummaryParam{{Text: tb.Thinking}},
				))
			}
			if m.Content != "" {
				input = append(input, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleAssistant,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: param.NewOpt(m.Content),
						},
					},
				})
			}
			for _, tu := range m.ToolUses {
				argsJSON, _ := json.Marshal(tu.Arguments)
				input = append(input, responses.ResponseInputItemUnionParam{
					OfFunctionCall: &responses.ResponseFunctionToolCallParam{
						Name:      tu.ToolName,
						CallID:    tu.ToolUseID,
						Arguments: string(argsJSON),
					},
				})
			}
		} else if len(m.ToolResults) > 0 {
			for _, tr := range m.ToolResults {
				input = append(input, responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: tr.ToolUseID,
						Output: tr.Content,
					},
				})
			}
		} else {
			input = append(input, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: param.NewOpt(m.Content),
					},
				},
			})
		}
	}
	return input
}

func classifyOpenAIError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 413 || (apiErr.StatusCode == 400 && containsContextLengthError(apiErr.Error())) {
			return &ContextTooLongError{Message: fmt.Sprintf("Context too long: %s", apiErr.Error())}
		}
		switch apiErr.StatusCode {
		case 401:
			return &AuthenticationError{Message: fmt.Sprintf("Invalid API key: %s", apiErr.Error())}
		case 429:
			retry := ""
			if apiErr.Response != nil {
				retry = apiErr.Response.Header.Get("Retry-After")
			}
			msg := "Rate limited."
			if retry != "" {
				msg += fmt.Sprintf(" Retry after %ss.", retry)
			} else {
				msg += " Please wait."
			}
			return &RateLimitError{Message: msg, RetryAfter: retry}
		default:
			return &LLMError{Message: fmt.Sprintf("API error (%d): %s", apiErr.StatusCode, apiErr.Error())}
		}
	}
	return &NetworkError{Message: fmt.Sprintf("Network error: %s", err.Error())}
}

func containsContextLengthError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "prompt is too long")
}
