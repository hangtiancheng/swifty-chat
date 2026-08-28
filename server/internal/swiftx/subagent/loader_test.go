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

package subagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAgentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-agent.md")
	content := `---
name: test-agent
description: A test agent for unit testing
disallowedTools:
  - EditFile
  - WriteFile
model: haiku
maxTurns: 25
---

You are a test agent. Do test things.`

	os.WriteFile(path, []byte(content), 0644)

	def, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.AgentType != "test-agent" {
		t.Errorf("AgentType = %q, want %q", def.AgentType, "test-agent")
	}
	if def.WhenToUse != "A test agent for unit testing" {
		t.Errorf("WhenToUse = %q", def.WhenToUse)
	}
	if len(def.DisallowedTools) != 2 {
		t.Errorf("DisallowedTools = %v, want 2 items", def.DisallowedTools)
	}
	if def.Model != "haiku" {
		t.Errorf("Model = %q, want %q", def.Model, "haiku")
	}
	if def.MaxTurns != 25 {
		t.Errorf("MaxTurns = %d, want 25", def.MaxTurns)
	}
	if def.SystemPrompt != "You are a test agent. Do test things." {
		t.Errorf("SystemPrompt = %q", def.SystemPrompt)
	}
}

func TestParseAgentFileMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.md")
	content := `---
description: No name field
---

Body text.`
	os.WriteFile(path, []byte(content), 0644)

	_, err := ParseAgentFile(path)
	if err == nil {
		t.Error("expected error for missing name, got nil")
	}
}

func TestParseAgentFileThirdPartyModelAllowed(t *testing.T) {
	// Matches AgentJsonSchema — model is "any non-empty string"; availability
	// is the ModelResolver's call, not the parser's. Used to be a hard
	// whitelist that silently broke definitions targeting GLM / OpenAI /
	// custom router names.
	dir := t.TempDir()
	path := filepath.Join(dir, "glm.md")
	content := `---
name: glm
description: Third-party model agent
model: glm-5.1
---

Body.`
	os.WriteFile(path, []byte(content), 0644)

	def, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("third-party model name must parse, got %v", err)
	}
	if def.Model != "glm-5.1" {
		t.Errorf("Model = %q, want %q", def.Model, "glm-5.1")
	}
}

func TestParseAgentFileInheritNormalization(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inherit.md")
	content := `---
name: inh
description: Mixed-case inherit
model: INHERIT
---

Body.`
	os.WriteFile(path, []byte(content), 0644)
	def, err := ParseAgentFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if def.Model != "inherit" {
		t.Errorf("INHERIT should normalize to inherit, got %q", def.Model)
	}
}

func TestLoaderBuiltinsAvailable(t *testing.T) {
	loader := NewAgentLoader(t.TempDir())
	loader.LoadAll()

	expected := []string{"explore", "general-purpose", "plan"}
	names := loader.ListNames()
	if len(names) != len(expected) {
		t.Fatalf("got %d agents, want %d: %v", len(names), len(expected), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, expected[i])
		}
	}

	gp := loader.Get("general-purpose")
	if gp == nil {
		t.Fatal("general-purpose not found")
	}
	if gp.Source != "built-in" {
		t.Errorf("Source = %q, want %q", gp.Source, "built-in")
	}
}

func TestLoaderRecordsFailedFilesAndWarns(t *testing.T) {
	// loadDir used to silently swallow parse errors, hiding the cause when a
	// user-edited definition broke. Failed files must now show up in
	// FailedFiles and on ErrorWriter.
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".swiftx", "agents")
	os.MkdirAll(agentsDir, 0o755)

	good := `---
name: ok-one
description: parses fine
---
body`
	bad := `---
name: missing-description
---
body`
	os.WriteFile(filepath.Join(agentsDir, "ok.md"), []byte(good), 0o644)
	os.WriteFile(filepath.Join(agentsDir, "bad.md"), []byte(bad), 0o644)

	var buf strings.Builder
	loader := NewAgentLoader(dir)
	loader.ErrorWriter = &buf
	loader.LoadAll()

	if loader.Get("ok-one") == nil {
		t.Error("ok-one should be loaded despite a sibling failure")
	}
	if len(loader.FailedFiles) != 1 {
		t.Fatalf("FailedFiles = %v, want 1 entry", loader.FailedFiles)
	}
	if !strings.Contains(loader.FailedFiles[0], "bad.md") {
		t.Errorf("FailedFiles entry should mention bad.md, got %q", loader.FailedFiles[0])
	}
	if !strings.Contains(buf.String(), "agent definition skipped") {
		t.Errorf("ErrorWriter should receive a warning, got: %q", buf.String())
	}
}

func TestParseAgentDefinitionExtendedFields(t *testing.T) {
	// All extended frontmatter fields must round-trip from YAML through ParseAgentFile into
	// AgentDefinition.
	dir := t.TempDir()
	path := filepath.Join(dir, "verify.md")
	content := `---
name: verify
description: extended-field smoke test
model: inherit
permissionMode: acceptEdits
background: true
isolation: worktree
memory: project
omitSwiftxMd: true
initialPrompt: "kick off with this"
skills: ["lint", "test"]
requiredMcpServers: ["github"]
maxTurns: 5
---
body`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	def, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("ParseAgentFile failed: %v", err)
	}
	if def.PermissionMode != "acceptEdits" {
		t.Errorf("PermissionMode = %q, want %q", def.PermissionMode, "acceptEdits")
	}
	if !def.Background {
		t.Error("Background should be true")
	}
	if def.Isolation != IsolationWorktree {
		t.Errorf("Isolation = %q, want %q", def.Isolation, IsolationWorktree)
	}
	if def.Memory != AgentMemoryScopeProject {
		t.Errorf("Memory = %q, want %q", def.Memory, AgentMemoryScopeProject)
	}
	if !def.OmitSwiftxMd {
		t.Error("OmitSwiftxMd should be true")
	}
	if def.InitialPrompt != "kick off with this" {
		t.Errorf("InitialPrompt = %q", def.InitialPrompt)
	}
	if len(def.Skills) != 2 || def.Skills[0] != "lint" {
		t.Errorf("Skills = %v", def.Skills)
	}
	if len(def.RequiredMcpServers) != 1 || def.RequiredMcpServers[0] != "github" {
		t.Errorf("RequiredMcpServers = %v", def.RequiredMcpServers)
	}

	// ToSpec must forward the extended fields so runSync / runAsync see them.
	spec := def.ToSpec()
	if spec.PermissionMode != "acceptEdits" || !spec.Background || spec.Isolation != IsolationWorktree {
		t.Errorf("ToSpec did not forward extended fields: %+v", spec)
	}
	if spec.InitialPrompt != "kick off with this" {
		t.Errorf("ToSpec.InitialPrompt = %q, want %q", spec.InitialPrompt, "kick off with this")
	}
}

func TestParseAgentInvalidPermissionMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.md")
	content := `---
name: bad
description: invalid mode
permissionMode: bogus
---
body`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAgentFile(path); err == nil {
		t.Fatal("ParseAgentFile should reject invalid permissionMode")
	}
}

func TestHasRequiredMcpServers(t *testing.T) {
	def := &AgentDefinition{RequiredMcpServers: []string{"github", "slack"}}
	if !def.HasRequiredMcpServers([]string{"GitHub-Server", "slack-mcp", "filesystem"}) {
		t.Error("should match case-insensitive substring")
	}
	if def.HasRequiredMcpServers([]string{"github"}) {
		t.Error("should fail when slack is missing")
	}
	empty := &AgentDefinition{}
	if !empty.HasRequiredMcpServers(nil) {
		t.Error("no requirements should always pass")
	}
}

func TestLoaderProjectOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".swiftx", "agents")
	os.MkdirAll(agentsDir, 0755)

	content := `---
name: explore
description: Custom explore agent for this project
model: sonnet
maxTurns: 50
---

You are a custom explore agent.`

	os.WriteFile(filepath.Join(agentsDir, "explore.md"), []byte(content), 0644)

	loader := NewAgentLoader(dir)
	loader.LoadAll()

	explore := loader.Get("explore")
	if explore == nil {
		t.Fatal("explore not found")
	}
	if explore.Source != "project" {
		t.Errorf("Source = %q, want %q", explore.Source, "project")
	}
	if explore.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", explore.Model, "sonnet")
	}
	if explore.MaxTurns != 50 {
		t.Errorf("MaxTurns = %d, want 50", explore.MaxTurns)
	}
}

func TestAgentDefinitionToSpec(t *testing.T) {
	def := &AgentDefinition{
		AgentType:       "test",
		WhenToUse:       "testing",
		DisallowedTools: []string{"Bash"},
		Model:           "haiku",
		MaxTurns:        10,
		SystemPrompt:    "You are a test agent.",
	}

	spec := def.ToSpec()
	if spec.Name != "test" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Model != "haiku" {
		t.Errorf("Model = %q", spec.Model)
	}
	if spec.MaxTurns != 10 {
		t.Errorf("MaxTurns = %d", spec.MaxTurns)
	}
	if spec.SystemPromptOverride != "You are a test agent." {
		t.Errorf("SystemPromptOverride = %q", spec.SystemPromptOverride)
	}
}
