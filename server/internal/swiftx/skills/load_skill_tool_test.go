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

// Source: WeChat Official Account @xiaolincoding
// Backend interview website: xiaolincoding.com
// Agent website: xiaolinnote.com
// Resume templates: jianli.xiaolinnote.com

package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// writeSkill drops a SKILL.md into a temp directory and returns a skill root that can be loaded directly.
func writeSkill(t *testing.T, name, frontmatter, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\n" + frontmatter + "---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return root
}

// In fork mode the SOP body is handed to a sub-agent; the main conversation only receives the final result.
func TestLoadSkillToolForkRunsSubAgent(t *testing.T) {
	root := writeSkill(t, "audit-deps", "mode: fork\n", "Inspect go.mod and flag risky pins.")
	catalog, err := LoadFromDirectory(root)
	if err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	host := newStubHost(tools.NewRegistry())
	host.subAgentReply = "3 risky pins found"

	tool := &LoadSkillTool{Catalog: catalog, Host: host, ForkHost: host}
	res := tool.Execute(context.Background(), map[string]any{"name": "audit-deps"})

	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Output)
	}
	if res.Output != "3 risky pins found" {
		t.Errorf("Output = %q, want the sub-agent's final text", res.Output)
	}
	if strings.Contains(res.Output, "Inspect go.mod") {
		t.Error("SOP body leaked into the main conversation; fork should keep it isolated")
	}
	if !strings.Contains(host.subAgentBody, "Inspect go.mod") {
		t.Errorf("sub-agent did not receive the skill body, got %q", host.subAgentBody)
	}
	if len(host.activated) != 0 {
		t.Errorf("fork skill should not be activated inline, got %v", host.activated)
	}
}

// When the host has not wired up a sub-agent runtime (ForkHost is nil), fall back to inline so the tool still works.
func TestLoadSkillToolForkFallsBackWithoutForkHost(t *testing.T) {
	root := writeSkill(t, "audit-deps", "mode: fork\n", "Inspect go.mod and flag risky pins.")
	catalog, err := LoadFromDirectory(root)
	if err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	host := newStubHost(tools.NewRegistry())
	tool := &LoadSkillTool{Catalog: catalog, Host: host}
	res := tool.Execute(context.Background(), map[string]any{"name": "audit-deps"})

	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Inspect go.mod") {
		t.Errorf("fallback should return the SOP body, got %q", res.Output)
	}
	if _, ok := host.activated["audit-deps"]; !ok {
		t.Error("fallback should activate the skill inline")
	}
}

// Inline mode keeps the original behavior: return the SOP body and register the activation.
func TestLoadSkillToolInlineReturnsBody(t *testing.T) {
	root := writeSkill(t, "commit", "mode: inline\n", "Write a conventional commit message.")
	catalog, err := LoadFromDirectory(root)
	if err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	host := newStubHost(tools.NewRegistry())
	tool := &LoadSkillTool{Catalog: catalog, Host: host, ForkHost: host}
	res := tool.Execute(context.Background(), map[string]any{"name": "commit"})

	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Write a conventional commit message.") {
		t.Errorf("inline should return the SOP body, got %q", res.Output)
	}
	if host.subAgentBody != "" {
		t.Error("inline skill must not spawn a sub-agent")
	}
}

// The legacy `context: fork` syntax is equivalent to `mode: fork`, so skills imported from other ecosystems also run in isolation correctly.
func TestLoadSkillToolLegacyContextForkRunsSubAgent(t *testing.T) {
	root := writeSkill(t, "audit-deps", "context: fork\n", "Inspect go.mod and flag risky pins.")
	catalog, err := LoadFromDirectory(root)
	if err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	host := newStubHost(tools.NewRegistry())
	host.subAgentReply = "done"

	tool := &LoadSkillTool{Catalog: catalog, Host: host, ForkHost: host}
	res := tool.Execute(context.Background(), map[string]any{"name": "audit-deps"})

	if res.Output != "done" {
		t.Errorf("Output = %q, want the sub-agent's final text", res.Output)
	}
	if host.subAgentBody == "" {
		t.Error("legacy context: fork should also run in a sub-agent")
	}
}
