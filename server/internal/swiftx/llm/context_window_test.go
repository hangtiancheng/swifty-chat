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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
)

// TestResolveContextWindow_FetchSuccess covers layer 2 working: a healthy
// /v1/models/{model} endpoint returns max_input_tokens, which is cached on the
// provider config and surfaced by GetContextWindow.
func TestResolveContextWindow_FetchSuccess(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"claude-sonnet-4-6","type":"model","display_name":"x","max_input_tokens":555000,"max_tokens":8192}`))
	}))
	defer srv.Close()

	cfg := &config.ProviderConfig{
		Protocol: "anthropic", BaseURL: srv.URL, APIKey: "k", Model: "claude-sonnet-4-6",
	}
	ResolveContextWindow(context.Background(), cfg)

	if !strings.Contains(gotPath, "/v1/models/claude-sonnet-4-6") {
		t.Errorf("expected fetch to hit /v1/models/{model}, got path %q", gotPath)
	}
	if got := cfg.GetContextWindow(); got != 555000 {
		t.Fatalf("fetched window should be used: got %d, want 555000", got)
	}
}

// TestResolveContextWindow_FetchErrorDegrades covers the critical path: when
// the endpoint errors (here 500), the fetch must fail silently — no panic, no
// blocking — and GetContextWindow falls back to the mapping table.
func TestResolveContextWindow_FetchErrorDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	cfg := &config.ProviderConfig{
		Protocol: "anthropic", BaseURL: srv.URL, APIKey: "k", Model: "claude-sonnet-4-6",
	}
	ResolveContextWindow(context.Background(), cfg) // must not panic

	// Mapping table gives 200000 for claude; the failed fetch must not lower it.
	if got := cfg.GetContextWindow(); got != 200000 {
		t.Fatalf("on fetch error should fall back to mapping table: got %d, want 200000", got)
	}
}

// TestResolveContextWindow_UnreachableDegrades simulates a dead endpoint
// (closed server). The bounded-timeout fetch must degrade to the mapping table
// without hanging or crashing.
func TestResolveContextWindow_UnreachableDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // close immediately so connections are refused

	cfg := &config.ProviderConfig{
		Protocol: "anthropic", BaseURL: url, APIKey: "k", Model: "gpt-4o",
	}
	ResolveContextWindow(context.Background(), cfg)

	if got := cfg.GetContextWindow(); got != 128000 {
		t.Fatalf("unreachable endpoint should fall back: got %d, want 128000", got)
	}
}

// TestResolveContextWindow_NonAnthropicSkipped confirms layer 2 only applies to
// Anthropic-protocol providers: a non-anthropic provider is never fetched and
// resolves via the mapping table.
func TestResolveContextWindow_NonAnthropicSkipped(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"max_input_tokens":999999}`))
	}))
	defer srv.Close()

	cfg := &config.ProviderConfig{
		Protocol: "openai-compat", BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o",
	}
	ResolveContextWindow(context.Background(), cfg)

	if called {
		t.Error("non-anthropic provider must not trigger a /v1/models fetch")
	}
	if got := cfg.GetContextWindow(); got != 128000 {
		t.Fatalf("non-anthropic should use mapping table: got %d, want 128000", got)
	}
}

// TestResolveContextWindow_ConfigOverrideSkipsFetch confirms that an explicit
// config window short-circuits the fetch entirely (no network call).
func TestResolveContextWindow_ConfigOverrideSkipsFetch(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"max_input_tokens":999999}`))
	}))
	defer srv.Close()

	cfg := &config.ProviderConfig{
		Protocol: "anthropic", BaseURL: srv.URL, APIKey: "k",
		Model: "claude-sonnet-4-6", ContextWindow: 4096,
	}
	ResolveContextWindow(context.Background(), cfg)

	if called {
		t.Error("explicit config window must skip the fetch")
	}
	if got := cfg.GetContextWindow(); got != 4096 {
		t.Fatalf("config window should win: got %d, want 4096", got)
	}
}
