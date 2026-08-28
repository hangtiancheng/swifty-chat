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

package teams

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileMailBoxSendAndRead(t *testing.T) {
	dir := t.TempDir()
	inboxDir := filepath.Join(dir, "test-team", "inboxes")
	mb := NewFileMailBox(inboxDir)

	// Send message
	err := mb.Send("agent-b", FileMailMessage{From: "agent-a", Text: "Hello from A"})
	if err != nil {
		t.Fatal("Send failed:", err)
	}

	// Verify file
	data, err := os.ReadFile(filepath.Join(inboxDir, "agent-b.json"))
	if err != nil {
		t.Fatal("File not created:", err)
	}
	var msgs []FileMailMessage
	json.Unmarshal(data, &msgs)
	if len(msgs) != 1 || msgs[0].From != "agent-a" || msgs[0].Text != "Hello from A" || msgs[0].Read {
		t.Fatalf("Unexpected content: %+v", msgs)
	}
}

func TestFileMailBoxReadUnread(t *testing.T) {
	dir := t.TempDir()
	mb := NewFileMailBox(filepath.Join(dir, "inboxes"))

	mb.Send("bob", FileMailMessage{From: "alice", Text: "msg1"})
	mb.Send("bob", FileMailMessage{From: "carol", Text: "msg2"})

	unread, err := mb.ReadUnread("bob")
	if err != nil || len(unread) != 2 {
		t.Fatalf("Expected 2 unread, got %d, err=%v", len(unread), err)
	}
}

func TestFileMailBoxMarkAllRead(t *testing.T) {
	dir := t.TempDir()
	mb := NewFileMailBox(filepath.Join(dir, "inboxes"))

	mb.Send("bob", FileMailMessage{From: "alice", Text: "msg1"})
	mb.Send("bob", FileMailMessage{From: "carol", Text: "msg2"})

	mb.MarkAllRead("bob")

	unread, _ := mb.ReadUnread("bob")
	if len(unread) != 0 {
		t.Fatalf("Expected 0 unread after mark, got %d", len(unread))
	}

	// Messages still in file
	data, _ := os.ReadFile(filepath.Join(dir, "inboxes", "bob.json"))
	var msgs []FileMailMessage
	json.Unmarshal(data, &msgs)
	if len(msgs) != 2 || !msgs[0].Read || !msgs[1].Read {
		t.Fatalf("Messages should be marked read: %+v", msgs)
	}
}

func TestFileMailBoxNonexistentAgent(t *testing.T) {
	dir := t.TempDir()
	mb := NewFileMailBox(filepath.Join(dir, "inboxes"))

	unread, err := mb.ReadUnread("nobody")
	if err != nil || len(unread) != 0 {
		t.Fatalf("Expected empty for nonexistent, got %d, err=%v", len(unread), err)
	}
}

func TestTeamSendMessageIntegration(t *testing.T) {
	dir := t.TempDir()
	team := NewTeam("test-team", ModeInProcess)
	team.MailBox = NewFileMailBox(filepath.Join(dir, "inboxes"))

	team.SendMessage("leader", "worker", "do task X")

	unread, _ := team.MailBox.ReadUnread("worker")
	if len(unread) != 1 || unread[0].From != "leader" || unread[0].Text != "do task X" {
		t.Fatalf("Unexpected: %+v", unread)
	}
}

func TestInjectPendingMessages(t *testing.T) {
	dir := t.TempDir()
	team := NewTeam("test-team", ModeInProcess)
	team.MailBox = NewFileMailBox(filepath.Join(dir, "inboxes"))

	team.SendMessage("alice", "bob", "hello bob")
	team.SendMessage("carol", "bob", "hey bob")

	result := InjectPendingMessages(team, "bob")
	if result == "" {
		t.Fatal("Expected non-empty result")
	}
	if !contains(result, "From alice: hello bob") || !contains(result, "From carol: hey bob") {
		t.Fatalf("Missing messages in result: %s", result)
	}

	// After inject, should be empty
	result2 := InjectPendingMessages(team, "bob")
	if result2 != "" {
		t.Fatalf("Expected empty after consume, got: %s", result2)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
