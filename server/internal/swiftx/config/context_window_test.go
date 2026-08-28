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

package config

import "testing"

// TestGetContextWindow_ConfigWins verifies layer 1: an explicit config value
// always takes top priority over every other layer (fetched / mapping table /
// default), regardless of the model name.
func TestGetContextWindow_ConfigWins(t *testing.T) {
	p := &ProviderConfig{Model: "claude-sonnet-4-6", ContextWindow: 12345}
	if got := p.GetContextWindow(); got != 12345 {
		t.Fatalf("config value should win: got %d, want 12345", got)
	}

	// Even if a fetched value is present, config still wins.
	p.SetFetchedContextWindow(999999)
	if got := p.GetContextWindow(); got != 12345 {
		t.Fatalf("config value should beat fetched value: got %d, want 12345", got)
	}
}

// TestGetContextWindow_FetchedBeatsMapping verifies layer 2 sits above the
// mapping table and default: a cached fetched value wins when config is unset.
func TestGetContextWindow_FetchedBeatsMapping(t *testing.T) {
	p := &ProviderConfig{Model: "claude-sonnet-4-6"} // mapping table would give 200000
	p.SetFetchedContextWindow(321000)
	if got := p.GetContextWindow(); got != 321000 {
		t.Fatalf("fetched value should beat mapping table: got %d, want 321000", got)
	}

	// A non-positive fetched value is ignored (failed fetch never pollutes).
	p2 := &ProviderConfig{Model: "gpt-4o"}
	p2.SetFetchedContextWindow(0)
	if got := p2.GetContextWindow(); got != 128000 {
		t.Fatalf("zero fetched value should be ignored: got %d, want 128000", got)
	}
}

// TestGetContextWindow_MappingTable verifies layer 3: the built-in
// model-name → window substring mapping returns the expected value for each
// model family, and falls through to the conservative default otherwise.
func TestGetContextWindow_MappingTable(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		// 1m substring (most specific) — beats the bare "claude" match.
		{"claude-sonnet-4-6-1m", 1000000},
		{"claude-opus-4-6-1m", 1000000},
		{"some-model-1m", 1000000},
		// OpenAI families.
		{"gpt-4.1", 1000000},
		{"gpt-4.1-mini", 1000000},
		{"gpt-4o", 128000},
		{"gpt-4o-mini", 128000},
		{"gpt-4-turbo", 128000},
		{"o1", 200000},
		{"o1-preview", 200000},
		{"o3-mini", 200000},
		{"o4-mini", 200000},
		{"gpt-3.5-turbo", 16385},
		// Claude generic.
		{"claude-sonnet-4-6", 200000},
		{"claude-3-5-haiku-20241022", 200000},
		// Unknown → conservative default.
		{"glm-4.7", 128000},
		{"some-unknown-model", 128000},
		{"", 128000},
	}
	for _, tt := range tests {
		p := &ProviderConfig{Model: tt.model}
		if got := p.GetContextWindow(); got != tt.want {
			t.Errorf("GetContextWindow(model=%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

// TestLookupModelContextWindow_NoMatch confirms the raw mapping lookup returns
// 0 (not the default) when nothing matches, so GetContextWindow can apply its
// own claude/other default split.
func TestLookupModelContextWindow_NoMatch(t *testing.T) {
	if got := lookupModelContextWindow("totally-unknown"); got != 0 {
		t.Fatalf("expected 0 for unknown model, got %d", got)
	}
}
