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
	"fmt"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

type Client interface {
	Stream(ctx context.Context, conv *conversation.Manager, tools []map[string]any) (<-chan StreamEvent, <-chan error)
	SetSystemPrompt(prompt string)
}

type MaxTokensSetter interface {
	SetMaxOutputTokens(tokens int)
}

func NewClient(cfg *config.ProviderConfig, systemPrompt string) (Client, error) {
	switch cfg.Protocol {
	case "anthropic":
		return newAnthropicClient(cfg, systemPrompt)
	case "openai":
		return newOpenAIClient(cfg, systemPrompt)
	case "openai-compat":
		return newOpenAICompatClient(cfg, systemPrompt)
	default:
		return nil, fmt.Errorf("unknown protocol: %s", cfg.Protocol)
	}
}

// contextWindowFetcher is implemented by clients that can pull the model's
// context window from their provider. Only the Anthropic client does so.
type contextWindowFetcher interface {
	FetchModelContextWindow(ctx context.Context) int
}

// ResolveContextWindow performs layer 2 of context-window resolution: for
// Anthropic-protocol providers it pulls the model's max_input_tokens from
// {base_url}/v1/models/{model} once and caches it on cfg via
// SetFetchedContextWindow, so later cfg.GetContextWindow() calls use it
// without hitting the network again.
//
// It is fully best-effort and never returns an error: a non-Anthropic
// provider, a client-construction failure, or a failed/timed-out fetch all
// leave the cache untouched, letting GetContextWindow fall back to the
// built-in mapping table / default. Safe to call at startup — it will not
// block beyond the fetch's own timeout and will not panic.
func ResolveContextWindow(ctx context.Context, cfg *config.ProviderConfig) {
	// An explicit config value already wins in GetContextWindow, so there's
	// nothing to fetch. Likewise skip if we've already cached a value.
	if cfg == nil || cfg.ContextWindow > 0 {
		return
	}
	if cfg.Protocol != "anthropic" {
		return
	}

	client, err := NewClient(cfg, "")
	if err != nil {
		return
	}
	fetcher, ok := client.(contextWindowFetcher)
	if !ok {
		return
	}
	if window := fetcher.FetchModelContextWindow(ctx); window > 0 {
		cfg.SetFetchedContextWindow(window)
	}
}
