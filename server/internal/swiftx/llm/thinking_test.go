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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

func fakeAnthropicSSE(w http.ResponseWriter, r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	io.WriteString(w, "event: message_start\n")
	io.WriteString(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"test","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":1}}}`+"\n\n")
	io.WriteString(w, "event: content_block_start\n")
	io.WriteString(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\n")
	io.WriteString(w, "event: content_block_delta\n")
	io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`+"\n\n")
	io.WriteString(w, "event: content_block_stop\n")
	io.WriteString(w, `data: {"type":"content_block_stop","index":0}`+"\n\n")
	io.WriteString(w, "event: message_delta\n")
	io.WriteString(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`+"\n\n")
	io.WriteString(w, "event: message_stop\n")
	io.WriteString(w, `data: {"type":"message_stop"}`+"\n\n")
	return body
}

func drainStream(client Client, conv *conversation.Manager) {
	events, errs := client.Stream(context.Background(), conv, nil)
	for range events {
	}
	select {
	case <-errs:
	default:
	}
}

func TestSupportsAdaptiveThinking(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-6", true},
		{"claude-sonnet-4-6-20250514", true},
		{"claude-opus-4-6", true},
		{"claude-opus-4-6-20250514", true},
		{"claude-opus-4-7", true},
		{"claude-sonnet-4-5-20250514", false},
		{"claude-sonnet-4-20250514", false},
		{"claude-3-5-sonnet-20241022", false},
		{"glm-4.7", false},
		{"some-unknown-model", false},
	}
	for _, tt := range tests {
		got := supportsAdaptiveThinking(tt.model)
		if got != tt.want {
			t.Errorf("supportsAdaptiveThinking(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestAnthropicThinkingAdaptive(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := fakeAnthropicSSE(w, r)
		json.Unmarshal(body, &captured)
	}))
	defer srv.Close()

	client, _ := newAnthropicClient(&config.ProviderConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "claude-sonnet-4-6", Thinking: true,
	}, "test system prompt")
	conv := conversation.NewManager()
	conv.AddUserMessage("hello")
	drainStream(client, conv)

	thinking, ok := captured["thinking"].(map[string]any)
	if !ok {
		t.Fatal("thinking field missing from request")
	}
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %q, want \"adaptive\"", thinking["type"])
	}
	if _, hasBudget := thinking["budget_tokens"]; hasBudget {
		t.Error("adaptive mode should not have budget_tokens")
	}
	t.Logf("Official model → adaptive: %+v", thinking)
}

func TestAnthropicThinkingEnabled(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := fakeAnthropicSSE(w, r)
		json.Unmarshal(body, &captured)
	}))
	defer srv.Close()

	client, _ := newAnthropicClient(&config.ProviderConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "glm-4.7", Thinking: true,
	}, "test system prompt")
	conv := conversation.NewManager()
	conv.AddUserMessage("hello")
	drainStream(client, conv)

	thinking, ok := captured["thinking"].(map[string]any)
	if !ok {
		t.Fatal("thinking field missing from request")
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %q, want \"enabled\"", thinking["type"])
	}
	budget, _ := thinking["budget_tokens"].(float64)
	if budget != 63999 {
		t.Errorf("budget_tokens = %v, want 63999 (maxTokens-1)", budget)
	}
	maxTokens, _ := captured["max_tokens"].(float64)
	if maxTokens != 64000 {
		t.Errorf("max_tokens = %v, want 64000", maxTokens)
	}
	t.Logf("Non-official model → enabled: %+v", thinking)
}

func TestAnthropicThinkingDisabled(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := fakeAnthropicSSE(w, r)
		json.Unmarshal(body, &captured)
	}))
	defer srv.Close()

	client, _ := newAnthropicClient(&config.ProviderConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "claude-sonnet-4-6", Thinking: false,
	}, "test system prompt")
	conv := conversation.NewManager()
	conv.AddUserMessage("hello")
	drainStream(client, conv)

	if _, ok := captured["thinking"]; ok {
		t.Error("thinking field should NOT be in API request when thinking=false")
	}
	maxTokens, _ := captured["max_tokens"].(float64)
	if maxTokens != 8192 {
		t.Errorf("max_tokens = %v, want 8192", maxTokens)
	}
}

func TestAnthropicThinkingBlocksInConversation(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := fakeAnthropicSSE(w, r)
		json.Unmarshal(body, &captured)
	}))
	defer srv.Close()

	client, _ := newAnthropicClient(&config.ProviderConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "claude-sonnet-4-6", Thinking: true,
	}, "test system prompt")
	conv := conversation.NewManager()
	conv.AddUserMessage("hello")
	conv.AddAssistantFull("hi there", []conversation.ThinkingBlock{
		{Thinking: "let me think about this", Signature: "sig123"},
	}, nil)
	conv.AddUserMessage("thanks")
	drainStream(client, conv)

	messages, _ := captured["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}

	assistantMsg, _ := messages[1].(map[string]any)
	content, _ := assistantMsg["content"].([]any)

	foundThinking := false
	for _, block := range content {
		blockMap, _ := block.(map[string]any)
		if blockMap["type"] == "thinking" {
			foundThinking = true
			if blockMap["thinking"] != "let me think about this" {
				t.Errorf("thinking text = %q, want %q", blockMap["thinking"], "let me think about this")
			}
			if blockMap["signature"] != "sig123" {
				t.Errorf("signature = %q, want %q", blockMap["signature"], "sig123")
			}
		}
	}
	if !foundThinking {
		body, _ := json.MarshalIndent(captured, "", "  ")
		t.Fatalf("no thinking block found in assistant message.\nRequest body:\n%s", string(body))
	}
}

func TestOpenAIThinkingEnabled(t *testing.T) {
	var captured map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		// minimal Responses API SSE
		lines := []string{
			`data: {"type":"response.output_text.delta","delta":"hi"}`,
			`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[]}}`,
		}
		io.WriteString(w, strings.Join(lines, "\n\n")+"\n\n")
	}))
	defer srv.Close()

	cfg := &config.ProviderConfig{
		Protocol: "openai",
		BaseURL:  srv.URL,
		APIKey:   "test-key",
		Model:    "o3",
		Thinking: true,
	}
	client, err := newOpenAIClient(cfg, "test system prompt")
	if err != nil {
		t.Fatal(err)
	}

	conv := conversation.NewManager()
	conv.AddUserMessage("hello")

	events, errs := client.Stream(context.Background(), conv, nil)
	for range events {
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatal(err)
		}
	default:
	}

	reasoning, ok := captured["reasoning"]
	if !ok {
		body, _ := json.MarshalIndent(captured, "", "  ")
		t.Fatalf("reasoning field missing from API request.\nFull request body:\n%s", string(body))
	}
	reasoningMap, ok := reasoning.(map[string]any)
	if !ok {
		t.Fatalf("reasoning is not an object: %T", reasoning)
	}
	effort, _ := reasoningMap["effort"].(string)
	if effort != "high" {
		t.Errorf("reasoning.effort = %q, want \"high\"", effort)
	}
	summary, _ := reasoningMap["summary"].(string)
	if summary != "detailed" {
		t.Errorf("reasoning.summary = %q, want \"detailed\"", summary)
	}

	include, _ := captured["include"].([]any)
	foundEncrypted := false
	for _, v := range include {
		if v == "reasoning.encrypted_content" {
			foundEncrypted = true
		}
	}
	if !foundEncrypted {
		t.Errorf("include should contain reasoning.encrypted_content, got: %v", include)
	}

	t.Logf("Request body reasoning: %+v", reasoning)
}

func TestOpenAIThinkingDisabled(t *testing.T) {
	var captured map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		lines := []string{
			`data: {"type":"response.output_text.delta","delta":"hi"}`,
			`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[]}}`,
		}
		io.WriteString(w, strings.Join(lines, "\n\n")+"\n\n")
	}))
	defer srv.Close()

	cfg := &config.ProviderConfig{
		Protocol: "openai",
		BaseURL:  srv.URL,
		APIKey:   "test-key",
		Model:    "gpt-4o",
		Thinking: false,
	}
	client, err := newOpenAIClient(cfg, "test system prompt")
	if err != nil {
		t.Fatal(err)
	}

	conv := conversation.NewManager()
	conv.AddUserMessage("hello")

	events, errs := client.Stream(context.Background(), conv, nil)
	for range events {
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatal(err)
		}
	default:
	}

	if _, ok := captured["reasoning"]; ok {
		t.Error("reasoning field should NOT be in API request when thinking=false")
	}
}
