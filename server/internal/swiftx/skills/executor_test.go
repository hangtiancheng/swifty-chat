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

package skills

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// stubHost captures every SkillHost call so tests can verify the executor
// fired the right side effects. Implements both SkillHost and SkillForkHost
// so the same fixture covers RunInline and RunFork.
type stubHost struct {
	activated     map[string]string
	registry      *tools.Registry
	parentMsgs    []conversation.Message
	subAgentBody  string
	subAgentSeed  []conversation.Message
	subAgentReply string
	subAgentErr   error
}

func newStubHost(reg *tools.Registry) *stubHost {
	return &stubHost{activated: map[string]string{}, registry: reg}
}

func (s *stubHost) ActivateSkill(name, body string) { s.activated[name] = body }
func (s *stubHost) ToolRegistry() *tools.Registry   { return s.registry }
func (s *stubHost) SnapshotParentMessages() []conversation.Message {
	out := make([]conversation.Message, len(s.parentMsgs))
	copy(out, s.parentMsgs)
	return out
}

func (s *stubHost) RunSubAgent(_ context.Context, body string, seed []conversation.Message, _ string) (string, error) {
	s.subAgentBody = body
	s.subAgentSeed = seed
	return s.subAgentReply, s.subAgentErr
}

func TestRunInlineActivates(t *testing.T) {
	reg := tools.NewRegistry()
	host := newStubHost(reg)
	skill := &Skill{
		Meta: SkillMeta{
			Name: "commit",
			Mode: "inline",
		},
		PromptBody: "Body with $ARGUMENTS",
		BodyLoaded: true,
	}

	body, err := RunInline(context.Background(), skill, "extra ctx", host)
	if err != nil {
		t.Fatalf("RunInline: %v", err)
	}
	if !strings.Contains(body, "extra ctx") {
		t.Errorf("body did not interpolate $ARGUMENTS: %q", body)
	}
	if host.activated["commit"] == "" {
		t.Errorf("ActivateSkill was not called")
	}
}

func TestRunForkPassesSeed(t *testing.T) {
	reg := tools.NewRegistry()
	host := newStubHost(reg)
	host.parentMsgs = []conversation.Message{
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "msg2"},
		{Role: "user", Content: "msg3"},
		{Role: "assistant", Content: "msg4"},
		{Role: "user", Content: "msg5"},
		{Role: "assistant", Content: "msg6"},
	}
	host.subAgentReply = "review complete"

	skill := &Skill{
		Meta: SkillMeta{
			Name:        "review",
			Mode:        "fork",
			ForkContext: "recent",
		},
		PromptBody: "Review this: $ARGUMENTS",
	}

	out, err := RunFork(context.Background(), skill, "main.go", host)
	if err != nil {
		t.Fatalf("RunFork: %v", err)
	}
	if out != "review complete" {
		t.Errorf("unexpected fork output: %q", out)
	}
	if !strings.Contains(host.subAgentBody, "main.go") {
		t.Errorf("sub-agent body missing args: %q", host.subAgentBody)
	}
	if len(host.subAgentSeed) != 5 {
		t.Errorf("recent seed should be last 5; got %d", len(host.subAgentSeed))
	}
	if host.subAgentSeed[0].Content != "msg2" {
		t.Errorf("seed should start at msg2 (last-5 window), got %q", host.subAgentSeed[0].Content)
	}
}

func TestRunForkContextNone(t *testing.T) {
	reg := tools.NewRegistry()
	host := newStubHost(reg)
	host.parentMsgs = []conversation.Message{
		{Role: "user", Content: "should not leak"},
	}
	skill := &Skill{
		Meta:       SkillMeta{Name: "isolated", Mode: "fork", ForkContext: "none"},
		PromptBody: "Pure isolation.",
	}
	_, err := RunFork(context.Background(), skill, "", host)
	if err != nil {
		t.Fatalf("RunFork: %v", err)
	}
	if len(host.subAgentSeed) != 0 {
		t.Errorf("none mode must seed nothing; got %d msgs", len(host.subAgentSeed))
	}
}

func TestRunForkPropagatesAgentError(t *testing.T) {
	reg := tools.NewRegistry()
	host := newStubHost(reg)
	host.subAgentErr = errors.New("upstream failure")

	skill := &Skill{Meta: SkillMeta{Name: "review", Mode: "fork"}}
	_, err := RunFork(context.Background(), skill, "", host)
	if err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("expected upstream failure, got %v", err)
	}
}

func TestLoadSkillToolReturnsConfirmation(t *testing.T) {
	reg := tools.NewRegistry()
	cat := NewCatalog()
	cat.Register(&Skill{
		Meta:       SkillMeta{Name: "commit", Mode: "inline"},
		PromptBody: "do commit stuff",
		BodyLoaded: true,
	}, "builtin")

	host := newStubHost(reg)
	tool := &LoadSkillTool{Catalog: cat, Host: host}
	res := tool.Execute(context.Background(), map[string]any{"name": "commit"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "# Skill: commit") || !strings.Contains(res.Output, "do commit stuff") {
		t.Errorf("expected full SOP body in output: %q", res.Output)
	}
	if host.activated["commit"] == "" {
		t.Errorf("ActivateSkill not called")
	}
}

func TestLoadSkillToolUnknown(t *testing.T) {
	reg := tools.NewRegistry()
	cat := NewCatalog()
	host := newStubHost(reg)
	tool := &LoadSkillTool{Catalog: cat, Host: host}

	res := tool.Execute(context.Background(), map[string]any{"name": "missing"})
	if !res.IsError {
		t.Fatalf("expected error for missing skill")
	}
}
