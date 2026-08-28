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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
)

// TestAnthropicUsageCacheParse verifies the Anthropic stream parser lifts
// cache_read_input_tokens / cache_creation_input_tokens out of the API usage
// into UsageInfo, alongside input/output. These are reported on message_start
// (input + cache_*) and finalized on message_delta (output), then accumulated
// by accMessage.
func TestAnthropicUsageCacheParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		io.WriteString(w, "event: message_start\n")
		io.WriteString(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"test","stop_reason":null,"usage":{"input_tokens":120,"output_tokens":1,"cache_read_input_tokens":5000,"cache_creation_input_tokens":200}}}`+"\n\n")
		io.WriteString(w, "event: content_block_start\n")
		io.WriteString(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\n")
		io.WriteString(w, "event: content_block_delta\n")
		io.WriteString(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`+"\n\n")
		io.WriteString(w, "event: content_block_stop\n")
		io.WriteString(w, `data: {"type":"content_block_stop","index":0}`+"\n\n")
		io.WriteString(w, "event: message_delta\n")
		io.WriteString(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":42}}`+"\n\n")
		io.WriteString(w, "event: message_stop\n")
		io.WriteString(w, `data: {"type":"message_stop"}`+"\n\n")
	}))
	defer srv.Close()

	client, _ := newAnthropicClient(&config.ProviderConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "claude-sonnet-4-6",
	}, "sys")
	conv := conversation.NewManager()
	conv.AddUserMessage("hello")

	events, errs := client.Stream(context.Background(), conv, nil)
	var end *StreamEnd
	for ev := range events {
		if e, ok := ev.(StreamEnd); ok {
			end = &e
		}
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
	default:
	}

	if end == nil {
		t.Fatal("no StreamEnd event received")
	}
	if end.Usage.InputTokens != 120 {
		t.Errorf("InputTokens = %d, want 120", end.Usage.InputTokens)
	}
	if end.Usage.OutputTokens != 42 {
		t.Errorf("OutputTokens = %d, want 42", end.Usage.OutputTokens)
	}
	if end.Usage.CacheReadTokens != 5000 {
		t.Errorf("CacheReadTokens = %d, want 5000", end.Usage.CacheReadTokens)
	}
	if end.Usage.CacheCreationTokens != 200 {
		t.Errorf("CacheCreationTokens = %d, want 200", end.Usage.CacheCreationTokens)
	}
}
