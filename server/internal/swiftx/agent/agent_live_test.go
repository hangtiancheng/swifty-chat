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

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/llm"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/prompt"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/skills"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// Live integration tests that hit the real LLM API.
// Run with: go test ./internal/agent/ -run TestLive -v -count=1
// Requires config.yaml in the project root with a valid API key.

func loadRealConfig(t *testing.T) *config.ProviderConfig {
	t.Helper()
	wd, _ := os.Getwd()
	for wd != "/" {
		if _, err := os.Stat(filepath.Join(wd, "config.yaml")); err == nil {
			break
		}
		wd = filepath.Dir(wd)
	}
	cfg, err := config.LoadConfig(filepath.Join(wd, "config.yaml"))
	if err != nil {
		t.Skipf("No config.yaml found: %v", err)
	}
	if len(cfg.Providers) == 0 {
		t.Skip("No providers configured")
	}
	p := &cfg.Providers[0]
	if p.ResolveAPIKey() == "" {
		t.Skip("No API key configured")
	}
	return p
}

func loadRealSkills(t *testing.T) (*skills.Catalog, string) {
	t.Helper()
	wd, _ := os.Getwd()
	for wd != "/" {
		if _, err := os.Stat(filepath.Join(wd, ".swiftx", "skills")); err == nil {
			break
		}
		wd = filepath.Dir(wd)
	}
	skillsDir := filepath.Join(wd, ".swiftx", "skills")
	catalog, err := skills.LoadFromDirectory(skillsDir)
	if err != nil {
		t.Skipf("Cannot load skills: %v", err)
	}
	return catalog, skillsDir
}

// liveRound sends a user message, runs the agent, and returns the text + all events.
// It auto-approves all tool permission requests.
func liveRound(t *testing.T, ag *Agent, conv *conversation.Manager, userMsg string) (string, []AgentEvent) {
	t.Helper()
	t.Logf(">>> USER: %s", truncate(userMsg, 200))
	conv.AddUserMessage(userMsg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	ch := ag.Run(ctx, conv)

	var events []AgentEvent
	var textParts []string

	for ev := range ch {
		switch e := ev.(type) {
		case PermissionRequestEvent:
			t.Logf("    [PERMISSION] %s: %s → auto-allow", e.ToolName, truncate(e.Desc, 80))
			e.ResponseCh <- PermAllow
			continue
		case StreamText:
			textParts = append(textParts, e.Text)
		case ThinkingText:
			// skip logging thinking to reduce noise
		case ToolUseEvent:
			if e.Args != nil {
				t.Logf("    [TOOL CALL] %s(%s)", e.ToolName, truncateArgs(e.Args))
			}
		case ToolResultEvent:
			status := "ok"
			if e.IsError {
				status = "ERROR"
			}
			t.Logf("    [TOOL RESULT] %s [%s] %.1fs: %s", e.ToolName, status, e.Elapsed.Seconds(), truncate(e.Output, 120))
		case ErrorEvent:
			t.Logf("    [ERROR] %s", e.Message)
		case TurnComplete:
			t.Logf("    [TURN %d complete]", e.Turn)
		case LoopComplete:
			t.Logf("    [LOOP complete, %d turns]", e.TotalTurns)
		}
		events = append(events, ev)
	}

	text := strings.Join(textParts, "")
	t.Logf("<<< AGENT: %s", truncate(text, 500))
	return text, events
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func truncateArgs(args map[string]any) string {
	var parts []string
	for k, v := range args {
		s := fmt.Sprintf("%v", v)
		if len(s) > 60 {
			s = s[:60] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return strings.Join(parts, ", ")
}

func TestLiveFrontendDesignSkill(t *testing.T) {
	providerCfg := loadRealConfig(t)
	catalog, skillsDir := loadRealSkills(t)

	skill := catalog.Get("frontend-design")
	if skill == nil {
		t.Skip("frontend-design skill not installed")
	}

	skillListing := buildSkillListing(skillsDir, catalog)
	client, err := llm.NewClient(providerCfg, skillListing)
	if err != nil {
		t.Fatalf("failed to create LLM client: %v", err)
	}

	workDir := t.TempDir()
	reg := tools.CreateDefaultRegistry()
	ag := New(client, reg, providerCfg.Protocol)
	ag.WorkDir = workDir
	conv := conversation.NewManager()

	// Round 1: invoke the skill with a specific and direct request
	skillPrompt := skill.PromptBody + "\n\n## User Request\n\nCreate a login page with email and password fields. " +
		"Use modern CSS, responsive design, and include aria-label attributes for accessibility. " +
		"Save it to " + filepath.Join(workDir, "login.html") + ". " +
		"Write the file directly."
	text1, _ := liveRound(t, ag, conv, skillPrompt)

	// The agent might ask questions or go straight to building.
	// If it asks questions, we respond. If it already built something, great.
	loginFile := filepath.Join(workDir, "login.html")
	if _, err := os.Stat(loginFile); os.IsNotExist(err) {
		// Agent probably asked a question or needs more info — respond
		if strings.Contains(strings.ToLower(text1), "?") || !fileExistsInDir(workDir) {
			text2, _ := liveRound(t, ag, conv,
				"Yes, please create a simple clean login page. Save it to "+loginFile+". Use modern CSS, make it responsive, include aria labels for accessibility.")
			t.Logf("Round 2 response length: %d chars", len(text2))
		}
	}

	// Check if a third round is needed
	if _, err := os.Stat(loginFile); os.IsNotExist(err) {
		// Maybe agent wrote to a different filename, check workDir
		files := listFiles(workDir)
		if len(files) == 0 {
			text3, _ := liveRound(t, ag, conv,
				"Please just write the HTML file now to "+loginFile)
			t.Logf("Round 3 response length: %d chars", len(text3))
		} else {
			t.Logf("Agent created files: %v (not login.html specifically)", files)
		}
	}

	// Final verification
	files := listFiles(workDir)
	t.Logf("Files in workDir: %v", files)

	if len(files) == 0 {
		t.Fatal("Agent did not create any files")
	}

	// Find and check the HTML file
	var htmlContent string
	for _, f := range files {
		if strings.HasSuffix(f, ".html") {
			content, err := os.ReadFile(filepath.Join(workDir, f))
			if err == nil {
				htmlContent = string(content)
				t.Logf("Found HTML file: %s (%d bytes)", f, len(content))
				break
			}
		}
	}

	if htmlContent == "" {
		t.Fatal("No HTML file was created by the agent")
	}

	// Content quality checks
	checks := map[string]string{
		"<html":    "valid HTML",
		"<form":    "has a form",
		"email":    "has email field",
		"password": "has password field",
		"<button":  "has submit button",
		"<style":   "has CSS styling",
	}
	passed := 0
	for substr, desc := range checks {
		if strings.Contains(strings.ToLower(htmlContent), strings.ToLower(substr)) {
			passed++
			t.Logf("  ✓ %s", desc)
		} else {
			t.Logf("  ✗ %s (missing %q)", desc, substr)
		}
	}
	t.Logf("Content checks: %d/%d passed", passed, len(checks))
	if passed < 4 {
		t.Errorf("Too few content checks passed (%d/%d), output quality too low", passed, len(checks))
	}

	// Log conversation summary
	msgs := conv.GetMessages()
	t.Logf("\n=== Conversation Summary (%d messages) ===", len(msgs))
	for i, m := range msgs {
		summary := truncate(m.Content, 100)
		extra := ""
		if len(m.ToolUses) > 0 {
			names := []string{}
			for _, tu := range m.ToolUses {
				names = append(names, tu.ToolName)
			}
			extra = fmt.Sprintf(" [tools: %s]", strings.Join(names, ", "))
		}
		if len(m.ToolResults) > 0 {
			extra = fmt.Sprintf(" [%d tool results]", len(m.ToolResults))
		}
		t.Logf("  [%d] %s: %s%s", i, m.Role, summary, extra)
	}
}

func TestLiveSkillCreatorOutputPath(t *testing.T) {
	providerCfg := loadRealConfig(t)
	catalog, _ := loadRealSkills(t)

	skill := catalog.Get("skill-creator")
	if skill == nil {
		t.Skip("skill-creator skill not installed")
	}

	// Use a temp dir to simulate a project with .swiftx/skills/
	workDir := t.TempDir()
	testSkillsDir := filepath.Join(workDir, ".swiftx", "skills")
	os.MkdirAll(testSkillsDir, 0o755)

	skillListing := buildSkillListing(testSkillsDir, catalog)
	client, err := llm.NewClient(providerCfg, skillListing)
	if err != nil {
		t.Fatalf("failed to create LLM client: %v", err)
	}

	reg := tools.CreateDefaultRegistry()
	ag := New(client, reg, providerCfg.Protocol)
	ag.WorkDir = workDir
	conv := conversation.NewManager()

	// Round 1: ask to create a simple skill
	prompt := skill.PromptBody + "\n\n## User Request\n\n" +
		"Create a simple skill called 'hello-world' that greets the user. " +
		"Just write the SKILL.md file directly, no evals needed. " +
		"The skill directory must be at: " + filepath.Join(testSkillsDir, "hello-world")
	text1, _ := liveRound(t, ag, conv, prompt)

	expectedDir := filepath.Join(testSkillsDir, "hello-world")
	expectedFile := filepath.Join(expectedDir, "SKILL.md")

	// Agent might ask questions first
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		liveRound(t, ag, conv,
			"Just create a minimal SKILL.md with frontmatter (name: hello-world, description: greet the user) "+
				"and a short body that says 'Greet the user warmly'. "+
				"Write it to "+expectedFile+". No tests, no evals, just the file.")
	}

	// Maybe one more nudge
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		liveRound(t, ag, conv,
			"Please write the file now to "+expectedFile)
	}

	// Verify the skill was created in the right place
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		// Check if it was created somewhere else
		allFiles := listFilesRecursive(workDir)
		t.Logf("All files in workDir: %v", allFiles)

		wrongLocation := false
		for _, f := range allFiles {
			if strings.Contains(f, "SKILL.md") && !strings.Contains(f, ".swiftx/skills/") {
				t.Errorf("SKILL.md created at WRONG location: %s (should be under .swiftx/skills/)", f)
				wrongLocation = true
			}
		}
		if !wrongLocation {
			t.Fatalf("SKILL.md not created at all. Expected: %s", expectedFile)
		}
		return
	}

	t.Logf("Skill file created at correct path: %s", expectedFile)

	// Verify it can be loaded
	content, _ := os.ReadFile(expectedFile)
	t.Logf("SKILL.md content:\n%s", string(content))

	updatedCatalog, err := skills.LoadFromDirectory(testSkillsDir)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	hw := updatedCatalog.Get("hello-world")
	if hw == nil {
		t.Error("hello-world skill not loadable after creation")
	} else {
		t.Logf("Skill loaded: name=%s desc=%s body=%d chars",
			hw.Meta.Name, truncate(hw.Meta.Description, 60), len(hw.PromptBody))
	}

	_ = text1
}

func TestLiveSimpleChat(t *testing.T) {
	providerCfg := loadRealConfig(t)

	env := prompt.DetectEnvironment(".")
	skillListing := prompt.BuildSystemPrompt(env)
	client, err := llm.NewClient(providerCfg, skillListing)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	reg := tools.CreateDefaultRegistry()
	ag := New(client, reg, providerCfg.Protocol)
	conv := conversation.NewManager()

	// Simple smoke test: just verify the API works
	text, _ := liveRound(t, ag, conv, "Reply with exactly: SWIFTX_OK")

	if !strings.Contains(text, "SWIFTX_OK") {
		t.Errorf("expected SWIFTX_OK in response, got: %s", truncate(text, 200))
	}
}

// --- helpers ---

func fileExistsInDir(dir string) bool {
	entries, _ := os.ReadDir(dir)
	return len(entries) > 0
}

func listFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func listFilesRecursive(root string) []string {
	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, rel)
		return nil
	})
	return files
}
