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

package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// splitCompoundCommand splits shell compound commands (&&, ||, ;, |) into
// independent sub-commands, matching permission rules one by one to prevent
// bypass via "cmd1 && dangerous_cmd".
func splitCompoundCommand(cmd string) []string {
	parts := regexp.MustCompile(`\s*(?:&&|\|\||[;|])\s*`).Split(cmd, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return []string{cmd}
	}
	return result
}

type DecisionEffect string

const (
	Allow DecisionEffect = "allow"
	Deny  DecisionEffect = "deny"
	Ask   DecisionEffect = "ask"
)

type Decision struct {
	Effect DecisionEffect
	Reason string
}

type PermissionMode string

const (
	ModeDefault     PermissionMode = "default"
	ModeAcceptEdits PermissionMode = "acceptEdits"
	ModePlan        PermissionMode = "plan"
	ModeBypass      PermissionMode = "bypassPermissions"
)

var modeMatrix = map[PermissionMode]map[tools.ToolCategory]DecisionEffect{
	ModeDefault:     {tools.CategoryRead: Allow, tools.CategoryWrite: Ask, tools.CategoryCommand: Ask},
	ModeAcceptEdits: {tools.CategoryRead: Allow, tools.CategoryWrite: Allow, tools.CategoryCommand: Ask},
	ModeBypass:      {tools.CategoryRead: Allow, tools.CategoryWrite: Allow, tools.CategoryCommand: Allow},
}

func ModeDecide(mode PermissionMode, category tools.ToolCategory) DecisionEffect {
	m, ok := modeMatrix[mode]
	if !ok {
		return Ask
	}
	return m[category]
}

// Layer 1: Dangerous command detection

type dangerousPattern struct {
	re     *regexp.Regexp
	reason string
}

var defaultDangerousPatterns = []dangerousPattern{
	{regexp.MustCompile(`rm\s+-[a-z]*r[a-z]*f[a-z]*\s+/\s*$`), "recursive force delete root"},
	{regexp.MustCompile(`mkfs\.`), "format disk"},
	{regexp.MustCompile(`dd\s+if=.*of=/dev/`), "direct write to disk device"},
	{regexp.MustCompile(`chmod\s+-R\s+777\s+/`), "recursive chmod root"},
	{regexp.MustCompile(`:\(\)\{\s*:\|:&\s*\};:`), "fork bomb"},
	{regexp.MustCompile(`curl\s+.*\|\s*(ba)?sh`), "pipe remote script"},
	{regexp.MustCompile(`wget\s+.*\|\s*(ba)?sh`), "pipe remote script"},
	{regexp.MustCompile(`>\s*/dev/sd`), "overwrite disk device"},
	// Git destructive commands — prevent accidental loss of work.
	{regexp.MustCompile(`git\s+push\s+.*--force`), "force push"},
	{regexp.MustCompile(`git\s+reset\s+--hard`), "hard reset"},
	{regexp.MustCompile(`git\s+clean\s+-f`), "force clean untracked files"},
	{regexp.MustCompile(`git\s+checkout\s+\.`), "discard all changes"},
	{regexp.MustCompile(`git\s+branch\s+-D`), "force delete branch"},
}

func DetectDangerous(command string) (bool, string) {
	for _, p := range defaultDangerousPatterns {
		if p.re.MatchString(command) {
			return true, p.reason
		}
	}
	return false, ""
}

// Layer 2: Path sandbox

type PathSandbox struct {
	allowedRoots []string
	denyWrite    []string // always read-only protected paths, higher priority than allowedRoots
}

// NewPathSandbox creates a path sandbox. denyWrite specifies protected paths
// (e.g. config files); writes are denied even within allowedRoots.
func NewPathSandbox(projectRoot string, extraAllowed ...string) *PathSandbox {
	root, _ := filepath.Abs(projectRoot)
	allowed := []string{root, os.TempDir()}
	for _, p := range extraAllowed {
		abs, _ := filepath.Abs(p)
		allowed = append(allowed, abs)
	}

	// Default protected paths: prevent Agent from tampering with permission
	// config and skill definitions.
	denyWrite := []string{
		filepath.Join(root, ".swiftx", "config.yaml"),
		filepath.Join(root, ".swiftx", "permissions.local.yaml"),
		filepath.Join(root, ".swiftx", "skills"),
	}

	return &PathSandbox{allowedRoots: allowed, denyWrite: denyWrite}
}

// CheckDenyWrite checks protected paths in isolation. These paths hold
// permission configuration and skill definitions; writes are denied under
// every permission mode, so callers must invoke it before the mode check.
func (s *PathSandbox) CheckDenyWrite(path string) (bool, string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Sprintf("cannot resolve path: %s", path)
	}
	for _, deny := range s.denyWrite {
		if abs == deny || strings.HasPrefix(abs, deny+string(filepath.Separator)) {
			return false, fmt.Sprintf("protected path: %s", path)
		}
	}
	return true, ""
}

func (s *PathSandbox) Check(path string) (bool, string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Sprintf("cannot resolve path: %s", path)
	}

	if ok, reason := s.CheckDenyWrite(path); !ok {
		return false, reason
	}

	for _, root := range s.allowedRoots {
		if strings.HasPrefix(abs, root) {
			return true, ""
		}
	}
	return false, fmt.Sprintf("path %s outside sandbox", path)
}

// GetDenyWrite returns the protected path list, used by sandbox Config construction.
func (s *PathSandbox) GetDenyWrite() []string {
	return s.denyWrite
}

// GetAllowedRoots returns the writable path list, used by sandbox Config construction.
func (s *PathSandbox) GetAllowedRoots() []string {
	return s.allowedRoots
}

// Layer 3: Rule engine

type RuleEffect string

const (
	RuleAllow RuleEffect = "allow"
	RuleDeny  RuleEffect = "deny"
	RuleAsk   RuleEffect = "ask"
)

type Rule struct {
	ToolName string
	Pattern  string
	Effect   RuleEffect
}

func (r Rule) Matches(toolName, content string) bool {
	if r.ToolName != toolName {
		return false
	}
	// Simple wildcard matching: * matches any character (including /), suitable
	// for Bash commands and other non-path scenarios. filepath.Match's * does
	// not match /, causing "allow always" to fail for commands containing paths.
	return globMatch(r.Pattern, content)
}

// globMatch implements simple wildcard matching where * matches any character
// (including /).
func globMatch(pattern, content string) bool {
	// Fast path: exact comparison when no wildcards are present.
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") {
		return pattern == content
	}
	// Convert pattern to regex: * → .*, ? → .
	var re strings.Builder
	re.WriteString("^")
	for _, ch := range pattern {
		switch ch {
		case '*':
			re.WriteString(".*")
		case '?':
			re.WriteString(".")
		case '.', '+', '^', '$', '{', '}', '(', ')', '|', '[', ']', '\\':
			re.WriteString("\\")
			re.WriteString(string(ch))
		default:
			re.WriteString(string(ch))
		}
	}
	re.WriteString("$")
	matched, _ := regexp.MatchString(re.String(), content)
	return matched
}

// cachedRules holds the parsed result of a single rule file. modTime and size
// together determine whether the file changed; comparing modTime alone is not
// enough, since consecutive writes within the same second may leave the
// timestamp unchanged on some filesystems.
type cachedRules struct {
	modTime time.Time
	size    int64
	rules   []Rule
}

type RuleEngine struct {
	UserPath    string
	ProjectPath string
	LocalPath   string

	// The background memory agent and the main agent may share the same
	// engine, so cache reads and writes must be locked.
	mu    sync.Mutex
	cache map[string]cachedRules
}

// NewRuleEngine builds a rule engine using conventional paths: the user-level
// file lives under the home directory, while the project-level and local-level
// files live under the working directory. When home cannot be resolved the
// user-level path is left empty and that layer is treated as having no rules.
func NewRuleEngine(workDir string) *RuleEngine {
	e := &RuleEngine{
		ProjectPath: filepath.Join(workDir, ".swiftx", "permissions.yaml"),
		LocalPath:   filepath.Join(workDir, ".swiftx", "permissions.local.yaml"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		e.UserPath = filepath.Join(home, ".swiftx", "permissions.yaml")
	}
	return e
}

// Evaluate merges the rules from the three rule files into a single set and
// returns the strictest effect among the matched rules. Priority is
// deny > ask > allow: which layer a rule lives in, or which line of the file
// it is on, does not affect the decision, so a single deny cannot be
// overridden by an allow in another layer. Returns nil when no rule matches.
func (e *RuleEngine) Evaluate(toolName, content string) *RuleEffect {
	return EvaluateRules(e.Snapshot(), toolName, content)
}

// Snapshot returns a merged snapshot of the three rule files. When a file has
// not changed, the previous parse result is reused; it is only re-read from
// disk when changed, so edits to a rule file take effect on the next
// evaluation without re-parsing on repeated evaluations. One snapshot is taken
// per tool call and shared when a compound command checks its sub-commands.
func (e *RuleEngine) Snapshot() []Rule {
	var all []Rule
	for _, path := range []string{e.UserPath, e.ProjectPath, e.LocalPath} {
		all = append(all, e.rulesFor(path)...)
	}
	return all
}

// rulesFor returns the rules of a single rule file, without reading from disk
// or parsing when the cache is hit.
func (e *RuleEngine) rulesFor(path string) []Rule {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		// The file does not exist or cannot be read; treat it as having no
		// rules and drop any stale cache entry.
		e.mu.Lock()
		delete(e.cache, path)
		e.mu.Unlock()
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.cache[path]; ok && c.modTime.Equal(info.ModTime()) && c.size == info.Size() {
		return c.rules
	}

	rules := loadRulesFile(path)
	if e.cache == nil {
		e.cache = make(map[string]cachedRules)
	}
	e.cache[path] = cachedRules{modTime: info.ModTime(), size: info.Size(), rules: rules}
	return rules
}

// EvaluateRules decides over the given rule set, with priority deny > ask >
// allow. Returns nil when no rule matches.
func EvaluateRules(rules []Rule, toolName, content string) *RuleEffect {
	var hit *RuleEffect
	for _, r := range rules {
		if !r.Matches(toolName, content) {
			continue
		}
		switch r.Effect {
		case RuleDeny:
			// deny is already the strictest effect and cannot be overridden;
			// return immediately.
			eff := RuleDeny
			return &eff
		case RuleAsk:
			eff := RuleAsk
			hit = &eff
		case RuleAllow:
			// allow is the weakest; record it only when no stricter effect
			// has been matched yet.
			if hit == nil {
				eff := RuleAllow
				hit = &eff
			}
		}
	}
	return hit
}

func (e *RuleEngine) AppendLocalRule(r Rule) {
	if e.LocalPath == "" {
		return
	}
	os.MkdirAll(filepath.Dir(e.LocalPath), 0o755)
	rules := loadRulesFile(e.LocalPath)
	rules = append(rules, r)
	var entries []map[string]string
	for _, rule := range rules {
		entries = append(entries, map[string]string{
			"rule":   fmt.Sprintf("%s(%s)", rule.ToolName, rule.Pattern),
			"effect": string(rule.Effect),
		})
	}
	data, _ := yaml.Marshal(entries)
	os.WriteFile(e.LocalPath, data, 0o644)
}

func loadRulesFile(path string) []Rule {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []struct {
		RuleStr string `yaml:"rule"`
		Effect  string `yaml:"effect"`
	}
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil
	}
	var rules []Rule
	for _, e := range entries {
		if e.Effect != "allow" && e.Effect != "deny" && e.Effect != "ask" {
			continue
		}
		r, err := parseRule(e.RuleStr, RuleEffect(e.Effect))
		if err != nil {
			continue
		}
		rules = append(rules, r)
	}
	return rules
}

var ruleRE = regexp.MustCompile(`^(\w+)\((.+)\)$`)

func parseRule(raw string, effect RuleEffect) (Rule, error) {
	m := ruleRE.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return Rule{}, fmt.Errorf("invalid rule syntax: %s", raw)
	}
	return Rule{ToolName: m[1], Pattern: m[2], Effect: effect}, nil
}

// Content extraction for rule matching

var contentFields = map[string]string{
	"Bash": "command", "ReadFile": "file_path", "WriteFile": "file_path",
	"EditFile": "file_path", "Glob": "pattern", "Grep": "pattern",
}

func ExtractContent(toolName string, args map[string]any) string {
	// McpCall's match target is not a single argument but "which MCP tool to
	// invoke", composed from the server + tool arguments as server__tool.
	// This allows rules like McpCall(linear__*) to allow/deny by server or
	// by tool.
	if toolName == tools.McpCallToolName {
		server, _ := args["server"].(string)
		tool, _ := args["tool"].(string)
		return tools.McpCallPermissionContent(server, tool)
	}
	field, ok := contentFields[toolName]
	if !ok {
		return ""
	}
	v, _ := args[field].(string)
	return v
}

// DescribeToolAction generates a human-readable action description for HITL
// confirmation. It extracts from standard content fields (command, file_path,
// etc.) first; falls back to a concatenated argument summary.
func DescribeToolAction(toolName string, args map[string]any) string {
	content := ExtractContent(toolName, args)
	if content != "" {
		return content
	}
	// No standard field available; build a short argument summary.
	var parts []string
	for k, v := range args {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:77] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	return toolName
}

// Layer 4+5: Permission Checker (orchestrates all layers)

type Checker struct {
	Sandbox      *PathSandbox
	RuleEngine   *RuleEngine
	Mode         PermissionMode
	PlanFilePath string
	// SandboxEnabled indicates whether the OS-level sandbox is active.
	// When enabled, Bash commands execute inside the sandbox; with autoAllow
	// confirmation can be skipped.
	SandboxEnabled bool
}

func NewChecker(sandbox *PathSandbox, ruleEngine *RuleEngine, mode PermissionMode) *Checker {
	return &Checker{
		Sandbox:    sandbox,
		RuleEngine: ruleEngine,
		Mode:       mode,
	}
}

func (c *Checker) Check(tool tools.Tool, args map[string]any) Decision {
	content := ExtractContent(tool.Name(), args)
	cat := tool.Category()
	// The rule snapshot is taken lazily once: safe commands, dangerous
	// commands, and the like return in earlier layers and never need to touch
	// the rule files; compound commands share the same snapshot when checking
	// sub-commands, avoiding repeated disk reads.
	snapshot := sync.OnceValue(c.RuleEngine.Snapshot)

	// Layer 0: Plan mode plan-file write exception
	if c.Mode == ModePlan && cat == tools.CategoryWrite && isPlanFile(content, c.PlanFilePath) {
		return Decision{Effect: Allow, Reason: "Plan mode: plan file write allowed"}
	}

	// Layer 1: safe read-only commands (auto-allow)
	if cat == tools.CategoryCommand && tools.IsSafeCommand(content) {
		return Decision{Effect: Allow, Reason: "Safe read-only command"}
	}

	// Layer 2: dangerous command (Bash only)
	// The blacklist is a hard line of defense: it must always be checked,
	// regardless of whether the sandbox is enabled.
	if cat == tools.CategoryCommand {
		hit, reason := DetectDangerous(content)
		if hit {
			return Decision{Effect: Deny, Reason: fmt.Sprintf("Dangerous command blocked: %s", reason)}
		}
	}

	// Layer 2b: sandbox auto-allow — commands running inside the OS sandbox
	// need no confirmation, but explicit deny/ask rules still apply.
	// Compound commands are split and checked individually; any sub-command
	// triggering deny causes overall deny, any triggering ask causes a prompt.
	if c.SandboxEnabled && cat == tools.CategoryCommand {
		subcommands := splitCompoundCommand(content)
		var hasAsk bool
		for _, sub := range subcommands {
			r := EvaluateRules(snapshot(), tool.Name(), sub)
			if r != nil && *r == RuleDeny {
				return Decision{Effect: Deny, Reason: "Permission rule: deny"}
			}
			if r != nil && *r == RuleAsk {
				hasAsk = true
			}
		}
		if hasAsk {
			return Decision{Effect: Ask, Reason: "Permission rule: ask (sandbox does not override explicit ask)"}
		}
		return Decision{Effect: Allow, Reason: "Sandboxed: auto-allow"}
	}

	// Layer 3: path sandbox (file tools)
	if (cat == tools.CategoryRead || cat == tools.CategoryWrite) && content != "" {
		// Protected paths are decided first: writes to permission config or
		// skill definitions are always denied, even in bypass mode.
		if cat == tools.CategoryWrite {
			if ok, reason := c.Sandbox.CheckDenyWrite(content); !ok {
				return Decision{Effect: Deny, Reason: reason}
			}
		}
		ok, reason := c.Sandbox.Check(content)
		if !ok {
			if c.Mode == ModeBypass {
				// bypass mode skips sandbox confirmation; allow directly.
			} else {
				return Decision{Effect: Ask, Reason: fmt.Sprintf("Path sandbox: %s", reason)}
			}
		}
	}

	// Layer 4: rule engine
	ruleResult := EvaluateRules(snapshot(), tool.Name(), content)
	if ruleResult != nil {
		switch *ruleResult {
		case RuleAllow:
			return Decision{Effect: Allow, Reason: "Permission rule: allow"}
		case RuleAsk:
			return Decision{Effect: Ask, Reason: "Permission rule: ask"}
		default:
			return Decision{Effect: Deny, Reason: "Permission rule: deny"}
		}
	}

	// Layer 4b: permission mode
	effect := ModeDecide(c.Mode, cat)
	if effect == Allow {
		return Decision{Effect: Allow, Reason: fmt.Sprintf("Permission mode %s: allow", c.Mode)}
	}
	if effect == Deny {
		return Decision{Effect: Deny, Reason: fmt.Sprintf("Permission mode %s: deny", c.Mode)}
	}

	// Layer 5: ASK → HITL
	return Decision{Effect: Ask, Reason: "User confirmation required"}
}

func isPlanFile(targetPath, planPath string) bool {
	if planPath == "" || targetPath == "" {
		return false
	}
	// Try absolute path comparison
	absTarget, err1 := filepath.Abs(targetPath)
	absPlan, err2 := filepath.Abs(planPath)
	if err1 == nil && err2 == nil && absTarget == absPlan {
		return true
	}
	// Check if target ends with the plan file's relative suffix
	cleanTarget := filepath.Clean(targetPath)
	cleanPlan := filepath.Clean(planPath)
	if cleanTarget == cleanPlan {
		return true
	}
	// Base name match: LLM occasionally shortens file_path to just the base name.
	// The plan slug is randomly generated (adjective+noun+timestamp), so collision
	// with an unrelated file under the same name is extremely unlikely.
	if filepath.Base(cleanTarget) == filepath.Base(cleanPlan) {
		return true
	}
	return false
}

// IsSafeCommand reports whether a command is a read-only safe command.
//
// The implementation lives in the tools package because the concurrency
// scheduler needs the same predicate (read-only commands may run alongside
// read-only tools), and tools must not depend on permissions. This is a
// thin forwarding wrapper so existing call sites in the rules layer are unchanged.
func IsSafeCommand(command string) bool { return tools.IsSafeCommand(command) }
