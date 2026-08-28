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
	"slices"
	"strings"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const anthropicStreamIdleTimeout = 5 * time.Minute

// nativeToolSearchBeta enables defer_loading and tool_reference. It holds the
// same value as mcp.NativeToolSearchBeta, defined here separately to avoid a
// reverse dependency from llm to mcp.
const nativeToolSearchBeta = "advanced-tool-use-2025-11-20"

// markToolsForCache places the cache breakpoint on the last non-deferred tool.
//
// Tool schemas are stable across turns, so marking the tail caches the entire tool block at
// essentially no cost. However, the breakpoint must not land on a tool with defer_loading: a tool
// carrying both defer_loading and cache_control causes the official endpoint to reject the entire
// request. MCP tools are registered after built-in tools, so after sorting the tail is often a
// deferred tool — hence the backward scan. Built-in tools are never deferred, so a valid landing
// spot always exists.
func markToolsForCache(sdkTools []anthropic.ToolUnionParam) {
	for i := len(sdkTools) - 1; i >= 0; i-- {
		t := sdkTools[i].OfTool
		if t == nil || t.DeferLoading.Valid() {
			continue
		}
		t.CacheControl = anthropic.NewCacheControlEphemeralParam()
		return
	}
}

// needsToolSearchBeta checks whether any tool in this batch carries defer_loading.
//
// The beta header is only sent when actually needed: endpoints that do not
// recognize it will reject the request outright, and the dispatch / eager
// paths do not need it at all.
func needsToolSearchBeta(toolSchemas []map[string]any) bool {
	for _, s := range toolSchemas {
		if deferLoading, _ := s["defer_loading"].(bool); deferLoading {
			return true
		}
	}
	return false
}

func supportsAdaptiveThinking(model string) bool {
	// claude-opus-4-6, claude-opus-4-7, claude-sonnet-4-6, etc.
	// but NOT claude-sonnet-4-5 (4.5 uses enabled mode)
	for _, family := range []string{"claude-opus-4-", "claude-sonnet-4-"} {
		if strings.HasPrefix(model, family) {
			rest := model[len(family):]
			if len(rest) > 0 && rest[0] >= '6' && rest[0] <= '9' {
				return true
			}
		}
	}
	return false
}

type anthropicClient struct {
	client          anthropic.Client
	model           string
	thinking        bool
	systemPrompt    string
	maxOutputTokens int
	contextWindow   int
}

func newAnthropicClient(cfg *config.ProviderConfig, systemPrompt string) (*anthropicClient, error) {
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		return nil, &AuthenticationError{
			Message: "Anthropic API key not found. Set it in .swiftx/config.yaml or via ANTHROPIC_API_KEY env var.",
		}
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(cfg.BaseURL),
	)

	return &anthropicClient{
		client:          client,
		model:           cfg.Model,
		thinking:        cfg.Thinking,
		systemPrompt:    systemPrompt,
		maxOutputTokens: cfg.GetMaxOutputTokens(),
		contextWindow:   cfg.GetContextWindow(),
	}, nil
}

func (c *anthropicClient) SetSystemPrompt(prompt string) {
	c.systemPrompt = prompt
}

func (c *anthropicClient) SetMaxOutputTokens(tokens int) {
	c.maxOutputTokens = tokens
}

// anthropicModelFetchTimeout bounds the auto-pull of model metadata so a slow
// or unreachable endpoint never delays startup.
const anthropicModelFetchTimeout = 3 * time.Second

// FetchModelContextWindow asks the Anthropic-compatible /v1/models/{model}
// endpoint for the model's max_input_tokens. It is best-effort: on any error
// (non-anthropic endpoint, network failure, timeout, missing field) it returns
// 0 and never panics or blocks beyond anthropicModelFetchTimeout. The caller
// treats 0 as "unknown" and falls back to the next context-window layer.
func (c *anthropicClient) FetchModelContextWindow(ctx context.Context) (window int) {
	// Hard guard: this runs at startup, so a panic in the SDK or a malformed
	// response must degrade silently rather than take the process down.
	defer func() {
		if recover() != nil {
			window = 0
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, anthropicModelFetchTimeout)
	defer cancel()

	// Best-effort startup call: disable retries so a flaky/failing endpoint
	// fails fast within the timeout instead of triggering a retry storm.
	info, err := c.client.Models.Get(ctx, c.model, anthropic.ModelGetParams{}, option.WithMaxRetries(0))
	if err != nil || info == nil || info.MaxInputTokens <= 0 {
		return 0
	}
	return int(info.MaxInputTokens)
}

func (c *anthropicClient) Stream(ctx context.Context, conv *conversation.Manager, toolSchemas []map[string]any) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent, 64)
	errs := make(chan error, 1)

	// Ensure tool_use/tool_result pairing before sending the request: interruptions,
	// session resumptions, and concurrent interleaving can leave dangling tool_use
	// blocks, which the API rejects outright if unpaired.
	msgs := buildAnthropicMessages(conversation.EnsureToolPairing(conv.GetMessages()))

	var sdkTools []anthropic.ToolUnionParam
	// Tools with defer_loading stay in tools[] but the server hides them from
	// the model; the model must first use ToolSearch to obtain a tool_reference
	// before calling them. This field requires the beta header to be accepted.
	sendToolSearchBeta := needsToolSearchBeta(toolSchemas)
	for _, s := range toolSchemas {
		inputSchema, _ := s["input_schema"].(map[string]any)
		props := inputSchema["properties"]
		required, _ := inputSchema["required"].([]string)
		desc, _ := s["description"].(string)
		tool := &anthropic.ToolParam{
			Name:        s["name"].(string),
			Description: param.NewOpt(desc),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: props,
				Required:   required,
			},
		}
		if deferLoading, _ := s["defer_loading"].(bool); deferLoading {
			tool.DeferLoading = param.NewOpt(true)
		}
		sdkTools = append(sdkTools, anthropic.ToolUnionParam{OfTool: tool})
	}

	go func() {
		defer close(events)
		defer close(errs)

		maxTokens := int64(c.maxOutputTokens)
		// Anchor the prompt cache on the longest-stable prefix: the system
		// prompt. Marked once here, plus once on the tool list and once on
		// the tail of the final user message below — Anthropic caches up to
		// each breakpoint and re-checks byte-identity on the next request.
		// tool_result content stays byte-stable past these breakpoints
		// because the toolresult budget finalizes each message at ingest
		// and never rewrites history afterwards.
		params := anthropic.MessageNewParams{
			Model:     c.model,
			MaxTokens: maxTokens,
			System: []anthropic.TextBlockParam{{
				Text:         c.systemPrompt,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			}},
			Messages: msgs,
		}
		markLastUserTailForCache(params.Messages)
		if c.thinking {
			if supportsAdaptiveThinking(c.model) {
				params.Thinking = anthropic.ThinkingConfigParamUnion{
					OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
				}
			} else {
				params.Thinking = anthropic.ThinkingConfigParamUnion{
					OfEnabled: &anthropic.ThinkingConfigEnabledParam{
						BudgetTokens: maxTokens - 1,
					},
				}
			}
		}
		if len(sdkTools) > 0 {
			markToolsForCache(sdkTools)
			params.Tools = sdkTools
		}

		var reqOpts []option.RequestOption
		if sendToolSearchBeta {
			reqOpts = append(reqOpts, option.WithHeaderAdd("anthropic-beta", nativeToolSearchBeta))
		}

		stream := c.client.Messages.NewStreaming(ctx, params, reqOpts...)
		defer stream.Close()

		var currentToolName, currentToolID, jsonAccum string
		var thinkingAccum, thinkingSignature string
		inThinking := false
		var accMessage anthropic.Message

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

		idle := time.NewTimer(anthropicStreamIdleTimeout)
		defer idle.Stop()

		go readNext()
		for {
			var res sseResult
			select {
			case <-ctx.Done():
				errs <- &NetworkError{Message: fmt.Sprintf("context cancelled: %v", ctx.Err())}
				return
			case <-idle.C:
				errs <- &NetworkError{Message: fmt.Sprintf("stream idle timeout: no SSE events for %s", anthropicStreamIdleTimeout)}
				return
			case res = <-nextCh:
			}

			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(anthropicStreamIdleTimeout)

			if !res.hasNext {
				break
			}

			event := stream.Current()
			accMessage.Accumulate(event)
			// Anthropic SDK's Accumulate only copies OutputTokens from
			// message_delta, but some providers (MiniMax) also report
			// InputTokens and cache fields there. Patch them in manually.
			if mde, ok := event.AsAny().(anthropic.MessageDeltaEvent); ok {
				if mde.Usage.InputTokens > 0 {
					accMessage.Usage.InputTokens = mde.Usage.InputTokens
				}
				if mde.Usage.CacheReadInputTokens > 0 {
					accMessage.Usage.CacheReadInputTokens = mde.Usage.CacheReadInputTokens
				}
				if mde.Usage.CacheCreationInputTokens > 0 {
					accMessage.Usage.CacheCreationInputTokens = mde.Usage.CacheCreationInputTokens
				}
			}
			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				switch ev.ContentBlock.Type {
				case "thinking":
					inThinking = true
					thinkingAccum = ""
					thinkingSignature = ""
				case "tool_use":
					currentToolName = ev.ContentBlock.Name
					currentToolID = ev.ContentBlock.ID
					jsonAccum = ""
					events <- ToolCallStart{ToolName: currentToolName, ToolID: currentToolID}
				}
			case anthropic.ContentBlockDeltaEvent:
				switch delta := ev.Delta.AsAny().(type) {
				case anthropic.ThinkingDelta:
					thinkingAccum += delta.Thinking
					events <- ThinkingDelta{Text: delta.Thinking}
				case anthropic.SignatureDelta:
					thinkingSignature = delta.Signature
				case anthropic.TextDelta:
					events <- TextDelta{Text: delta.Text}
				case anthropic.InputJSONDelta:
					jsonAccum += delta.PartialJSON
					events <- ToolCallDelta{Text: delta.PartialJSON}
				}
			case anthropic.ContentBlockStopEvent:
				if inThinking {
					events <- ThinkingComplete{
						Thinking:  thinkingAccum,
						Signature: thinkingSignature,
					}
					inThinking = false
				}
				if currentToolName != "" {
					var args map[string]any
					if jsonAccum != "" {
						json.Unmarshal([]byte(jsonAccum), &args)
					}
					if args == nil {
						args = map[string]any{}
					}
					events <- ToolCallComplete{
						ToolID:    currentToolID,
						ToolName:  currentToolName,
						Arguments: args,
					}
					currentToolName = ""
					currentToolID = ""
					jsonAccum = ""
				}
			}

			go readNext()
		}

		if err := stream.Err(); err != nil {
			errs <- classifyAnthropicError(err)
			return
		}

		stopReason := string(accMessage.StopReason)
		if stopReason == "" {
			stopReason = "end_turn"
		}
		usage := UsageInfo{
			InputTokens:         int(accMessage.Usage.InputTokens),
			OutputTokens:        int(accMessage.Usage.OutputTokens),
			CacheReadTokens:     int(accMessage.Usage.CacheReadInputTokens),
			CacheCreationTokens: int(accMessage.Usage.CacheCreationInputTokens),
		}
		events <- StreamEnd{StopReason: stopReason, Usage: usage}
	}()

	return events, errs
}

// markLastUserTailForCache attaches an ephemeral cache_control marker to the
// last content block of the final user-role message. Anthropic caches the
// prefix up to (and including) this block; subsequent requests with a
// byte-identical prefix hit the cache. tool_result content past this
// breakpoint stays byte-stable because the toolresult budget finalizes
// each message at ingest and never rewrites history afterwards.
//
// Mutates `messages` in place. No-op if there's no user message or the
// final user message has no content blocks we can mark.
func markLastUserTailForCache(messages []anthropic.MessageParam) {
	for _, v := range slices.Backward(messages) {
		if v.Role != anthropic.MessageParamRoleUser {
			continue
		}
		blocks := v.Content
		if len(blocks) == 0 {
			return
		}
		last := &blocks[len(blocks)-1]
		switch {
		case last.OfText != nil:
			last.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
		case last.OfToolResult != nil:
			last.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		return
	}
}

func buildAnthropicMessages(messages []conversation.Message) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	for _, m := range messages {
		if m.Role == "assistant" {
			var blocks []anthropic.ContentBlockParamUnion
			for _, tb := range m.ThinkingBlocks {
				blocks = append(blocks, anthropic.NewThinkingBlock(tb.Signature, tb.Thinking))
			}
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tu := range m.ToolUses {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tu.ToolUseID,
						Name:  tu.ToolName,
						Input: tu.Arguments,
					},
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropic.NewTextBlock(""))
			}
			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: blocks,
			})
		} else if len(m.ToolResults) > 0 {
			var blocks []anthropic.ContentBlockParamUnion
			for _, tr := range m.ToolResults {
				// Structured blocks go through the block array (tool_reference and
				// similar server-parsed content can only be sent this way);
				// everything else is sent as plain text as before
				content := []anthropic.ToolResultBlockParamContentUnion{{
					OfText: &anthropic.TextBlockParam{Text: tr.Content},
				}}
				if structured := toolResultContentBlocks(tr.ContentBlocks); len(structured) > 0 {
					content = structured
				}
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: tr.ToolUseID,
						IsError:   param.NewOpt(tr.IsError),
						Content:   content,
					},
				})
			}
			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: blocks,
			})
		} else {
			// Merge consecutive user text messages to maintain alternation.
			canMerge := false
			if n := len(result); n > 0 {
				prev := result[n-1]
				if prev.Role == anthropic.MessageParamRoleUser && len(prev.Content) > 0 && prev.Content[0].OfToolResult == nil {
					canMerge = true
				}
			}
			if canMerge {
				result[len(result)-1].Content = append(result[len(result)-1].Content, anthropic.NewTextBlock(m.Content))
			} else {
				result = append(result, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
			}
		}
	}
	return result
}

func classifyAnthropicError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 413 || strings.Contains(apiErr.Error(), "prompt is too long") {
			return &ContextTooLongError{Message: fmt.Sprintf("Context too long: %s", apiErr.Error())}
		}
		switch apiErr.Type() {
		case anthropic.ErrorTypeAuthenticationError:
			return &AuthenticationError{Message: fmt.Sprintf("Invalid API key: %s", apiErr.Error())}
		case anthropic.ErrorTypeRateLimitError:
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

// toolResultContentBlocks converts structured blocks produced by tools into the
// SDK's typed union. Only tool_reference is recognized — currently the only
// block type requiring server-side parsing; ToolSearch on the official endpoint
// uses it to have the server expand MCP tool schemas into context. Unrecognized
// blocks cause the entire conversion to be abandoned and the caller falls back
// to plain text — better to omit a block than to send a malformed request.
func toolResultContentBlocks(raw []map[string]any) []anthropic.ToolResultBlockParamContentUnion {
	if len(raw) == 0 {
		return nil
	}
	out := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(raw))
	for _, b := range raw {
		blockType, _ := b["type"].(string)
		if blockType != "tool_reference" {
			return nil
		}
		name, _ := b["tool_name"].(string)
		if name == "" {
			return nil
		}
		out = append(out, anthropic.ToolResultBlockParamContentUnion{
			OfToolReference: &anthropic.ToolReferenceBlockParam{ToolName: name},
		})
	}
	return out
}
