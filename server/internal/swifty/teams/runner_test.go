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
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMain points every teams test at a throwaway mailbox root so
// running the suite doesn't litter the repo with .swifty/teams/
// directories.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "swifty-teams-test-")
	if err != nil {
		panic(err)
	}
	// The team directory is <home>/.swifty/teams; redirect the entire home
	// directory to a temp dir so all package tests land in the sandbox.
	// Windows reads USERPROFILE; other platforms read HOME.
	_ = os.Setenv("HOME", tmp)
	_ = os.Setenv("USERPROFILE", tmp)
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func TestIsShutdownRequest(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"[shutdown] please stop", true},
		{"  [shutdown]  ", true},
		{"shutdown", false},
		{"hello [shutdown] there", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsShutdownRequest(FileMailMessage{Text: c.text}); got != c.want {
			t.Errorf("IsShutdownRequest(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestCreateIdleNotification(t *testing.T) {
	msg := CreateIdleNotification("alice", "available")
	if msg.From != "alice" {
		t.Errorf("From = %q, want alice", msg.From)
	}
	if !strings.Contains(msg.Text, "[idle]") {
		t.Errorf("Text missing [idle] marker: %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "alice") {
		t.Errorf("Text missing member name: %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "available") {
		t.Errorf("Text missing reason: %q", msg.Text)
	}
	if msg.Timestamp == "" {
		t.Error("Timestamp should be set")
	}
}

func TestFormatInboundAsPromptEmpty(t *testing.T) {
	if got := formatInboundAsPrompt(nil); got != "" {
		t.Errorf("empty input should yield empty prompt, got %q", got)
	}
}

func TestFormatInboundAsPromptMultiple(t *testing.T) {
	msgs := []FileMailMessage{
		{From: "lead", Text: "go review file X"},
		{From: "bob", Text: "I'll handle the tests"},
	}
	got := formatInboundAsPrompt(msgs)
	if !strings.Contains(got, "From lead: go review file X") {
		t.Errorf("missing first message: %q", got)
	}
	if !strings.Contains(got, "From bob: I'll handle the tests") {
		t.Errorf("missing second message: %q", got)
	}
	if !strings.Contains(got, "new messages from your team") {
		t.Errorf("missing header: %q", got)
	}
}

func TestWaitForNextPromptOrShutdownShutdown(t *testing.T) {
	dir := t.TempDir()
	team := &Team{
		Name:    "x",
		members: map[string]*Member{},
		MailBox: NewFileMailBox(dir),
	}

	// Drop a shutdown message and verify the wait returns immediately
	// with shutdown=true.
	if err := team.MailBox.Send("alice", FileMailMessage{From: LeadName, Text: "[shutdown] done"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	prompt, shutdown, err := waitForNextPromptOrShutdown(ctx, team, "alice")
	if err != nil {
		t.Fatalf("waitForNextPromptOrShutdown: %v", err)
	}
	if shutdown == nil {
		t.Errorf("expected a shutdown message, got nil")
	}
	if prompt != "" {
		t.Errorf("expected empty prompt on shutdown, got %q", prompt)
	}
}

func TestWaitForNextPromptOrShutdownMessage(t *testing.T) {
	dir := t.TempDir()
	team := &Team{
		Name:    "x",
		members: map[string]*Member{},
		MailBox: NewFileMailBox(dir),
	}

	if err := team.MailBox.Send("alice", FileMailMessage{From: LeadName, Text: "do the thing"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	prompt, shutdown, err := waitForNextPromptOrShutdown(ctx, team, "alice")
	if err != nil {
		t.Fatalf("waitForNextPromptOrShutdown: %v", err)
	}
	if shutdown != nil {
		t.Error("unexpected shutdown message on regular message")
	}
	if !strings.Contains(prompt, "do the thing") {
		t.Errorf("prompt missing message body: %q", prompt)
	}

	// Inbox should have been drained.
	leftover, _ := team.MailBox.ReadUnread("alice")
	if len(leftover) != 0 {
		t.Errorf("expected inbox drained, %d unread remain", len(leftover))
	}
}

func TestWaitForNextPromptOrShutdownCancel(t *testing.T) {
	dir := t.TempDir()
	team := &Team{
		Name:    "x",
		members: map[string]*Member{},
		MailBox: NewFileMailBox(dir),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call returns

	_, _, err := waitForNextPromptOrShutdown(ctx, team, "alice")
	if err == nil {
		t.Error("expected ctx error, got nil")
	}
}

func TestDrainLeadMailbox(t *testing.T) {
	// Build teams with explicit mailbox dirs so we don't pollute the
	// repo root via teamsBaseDir().
	mgr := NewTeamManager(t.TempDir())
	t1 := &Team{Name: "alpha", members: map[string]*Member{}, MailBox: NewFileMailBox(t.TempDir())}
	t2 := &Team{Name: "beta", members: map[string]*Member{}, MailBox: NewFileMailBox(t.TempDir())}
	mgr.CreateTeamWith(t1)
	mgr.CreateTeamWith(t2)

	_ = t1.MailBox.Send(LeadName, FileMailMessage{From: "ann", Text: "[idle] ann (reason: available)"})
	_ = t2.MailBox.Send(LeadName, FileMailMessage{From: "bob", Text: "[idle] bob (reason: failed)"})

	notes := DrainLeadMailbox(mgr)
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "team=\"alpha\"") || !strings.Contains(joined, "team=\"beta\"") {
		t.Errorf("notes missing team labels: %s", joined)
	}
	if !strings.Contains(joined, "ann") || !strings.Contains(joined, "bob") {
		t.Errorf("notes missing senders: %s", joined)
	}

	// Second drain should yield nothing because messages are now read.
	if again := DrainLeadMailbox(mgr); len(again) != 0 {
		t.Errorf("expected empty drain after mark-read, got %d", len(again))
	}
}

func TestDrainLeadMailboxNilSafe(t *testing.T) {
	if got := DrainLeadMailbox(nil); got != nil {
		t.Errorf("nil manager should yield nil, got %v", got)
	}
}

func TestSpawnTeammateValidation(t *testing.T) {
	ctx := context.Background()

	// Missing team
	if _, err := SpawnTeammate(ctx, TeammateSpawnConfig{MemberName: "x"}); err == nil {
		t.Error("expected error when Team is nil")
	}

	// Missing name
	team := NewTeam(t.TempDir(), "t")
	if _, err := SpawnTeammate(ctx, TeammateSpawnConfig{Team: team}); err == nil {
		t.Error("expected error when MemberName is empty")
	}
}
