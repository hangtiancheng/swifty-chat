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

package tui

import (
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
)

func TestClearResetsSession(t *testing.T) {
	provider := config.ProviderConfig{
		Name:     "test",
		Protocol: "anthropic",
		Model:    "claude-sonnet-4-20250514",
		APIKey:   "test-key",
	}
	m := New([]config.ProviderConfig{provider}, nil, nil)

	// Simulate some existing state
	m.chatMessages = []chatMessage{
		{role: "user", content: "hello"},
		{role: "assistant", content: "hi"},
	}
	m.committedUpTo = 2
	m.totalInput = 1000
	m.totalOutput = 500
	oldSessionID := m.sessionID

	// Execute /clear
	updated, _ := m.executeCommand("clear", "")
	m = updated.(Model)

	// Verify messages are cleared
	if len(m.chatMessages) != 0 {
		t.Errorf("chatMessages should be empty after /clear, got %d", len(m.chatMessages))
	}

	// Verify committedUpTo is reset to zero
	if m.committedUpTo != 0 {
		t.Errorf("committedUpTo should be 0 after /clear, got %d", m.committedUpTo)
	}

	// Verify session ID has changed (differs from old value)
	if m.sessionID == oldSessionID {
		t.Errorf("sessionID should change after /clear, still %s", m.sessionID)
	}
	if m.sessionID == "" {
		t.Error("sessionID should not be empty after /clear")
	}

	// Verify token counts are reset to zero
	if m.totalInput != 0 {
		t.Errorf("totalInput should be 0 after /clear, got %d", m.totalInput)
	}
	if m.totalOutput != 0 {
		t.Errorf("totalOutput should be 0 after /clear, got %d", m.totalOutput)
	}

	// Verify conversation has been reconstructed
	if m.conversation == nil {
		t.Error("conversation should not be nil after /clear")
	}
}
