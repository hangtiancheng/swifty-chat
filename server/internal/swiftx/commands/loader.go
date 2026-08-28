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

package commands

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CommandMeta is the frontmatter for a file-based prompt command. Holds the subset of fields read
// for legacy /commands/ files: description, argument-hint, aliases.
type CommandMeta struct {
	Description  string   `yaml:"description"`
	ArgumentHint string   `yaml:"argument-hint"`
	Aliases      []string `yaml:"aliases"`
}

// LoadDir scans dir for *.md files (recursive) and returns one Command per file. The command name
// is derived from the file's path relative to dir, with subdirectories joined by ':' for original
// namespacing rule (sub/dir/foo.md → "sub:dir:foo"). Files that fail to parse are silently skipped
// (a malformed user command should not break startup).
//
// Each returned command has Type=TypePrompt and a Handler that returns the markdown body with
// $ARGUMENTS substituted. If the body has no $ARGUMENTS placeholder and args is non-empty, the args
// are appended in a "## User Request" section.
func LoadDir(dir string) []*Command {
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	var cmds []*Command
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		cmd := parseCommandFile(dir, path)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return nil
	})
	return cmds
}

// LoadUserCommands merges file-based commands from two search paths: 1. ~/.swiftx/commands/ (user
// global) 2. $workDir/.swiftx/commands/ (project).
//
// Later sources override earlier ones on name collision.
func LoadUserCommands(workDir string) []*Command {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".swiftx", "commands"))
	}
	dirs = append(dirs,
		filepath.Join(workDir, ".swiftx", "commands"),
	)

	merged := map[string]*Command{}
	var order []string
	for _, d := range dirs {
		for _, cmd := range LoadDir(d) {
			if _, seen := merged[cmd.Name]; !seen {
				order = append(order, cmd.Name)
			}
			merged[cmd.Name] = cmd
		}
	}

	out := make([]*Command, 0, len(order))
	for _, name := range order {
		out = append(out, merged[name])
	}
	return out
}

// parseCommandFile reads a single .md file and returns the matching Command, or nil on read/parse
// failure. The name is computed from the relative path: "git/log.md" under baseDir → "git:log".
// Names are lowercased to match the /<name> lookup convention in Parse.
func parseCommandFile(baseDir, path string) *Command {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return nil
	}
	rel = strings.TrimSuffix(rel, ".md")
	parts := strings.Split(rel, string(filepath.Separator))
	for i, p := range parts {
		parts[i] = strings.ToLower(strings.ReplaceAll(p, " ", "-"))
	}
	name := strings.Join(parts, ":")
	if name == "" {
		return nil
	}

	meta, body := splitFrontmatter(string(data))
	body = strings.TrimSpace(body)
	if meta.Description == "" {
		meta.Description = firstNonHeaderLine(body)
	}

	return &Command{
		Name:        name,
		Description: meta.Description,
		Aliases:     meta.Aliases,
		Type:        TypePrompt,
		ArgPrompt:   meta.ArgumentHint,
		Handler:     promptHandler(body),
	}
}

// splitFrontmatter separates YAML frontmatter from the markdown body. Returns an empty meta and the
// original content if no frontmatter is present or it fails to parse — a malformed command file
// should not break startup.
func splitFrontmatter(content string) (CommandMeta, string) {
	var meta CommandMeta
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return meta, content
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return meta, content
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return CommandMeta{}, content
	}
	return meta, parts[2]
}

// firstNonHeaderLine returns the first non-empty, non-heading line — used as a description fallback
// when frontmatter doesn't supply one.
func firstNonHeaderLine(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

// promptHandler returns a Handler that renders the command body with $ARGUMENTS substitution.
// Bodies without a placeholder append the args in a "## User Request" section.
func promptHandler(body string) Handler {
	return func(ctx *Context) string {
		if strings.Contains(body, "$ARGUMENTS") {
			return strings.ReplaceAll(body, "$ARGUMENTS", ctx.Args)
		}
		if strings.TrimSpace(ctx.Args) == "" {
			return body
		}
		return body + "\n\n## User Request\n\n" + ctx.Args
	}
}
