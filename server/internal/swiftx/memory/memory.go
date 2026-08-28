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

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Manager wraps the dual auto-memory directories (user-level + project-level).
// It is a thin coordinator: the actual save/load happens via the agent's
// Write/Read tools (per Swiftx's reference architecture). This struct exists
// to give the TUI a stable handle for system-prompt building and for the
// `/memory` slash command (list / clear).
type Manager struct {
	projectRoot string
	userMemDir  string // ~/.swiftx/memory/ — user/feedback type memories
	memDir      string // <projectRoot>/.swiftx/memory/ — project/reference type memories
}

// NewManager creates a Manager for the given project root. Resolves both
// the user-level and project-level memory directories. Either may be empty
// (e.g. user-level resolves empty if $HOME is unset).
func NewManager(projectRoot string) *Manager {
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		abs = projectRoot
	}
	return &Manager{
		projectRoot: abs,
		userMemDir:  GetUserAutoMemPath(),
		memDir:      GetAutoMemPath(abs),
	}
}

// Dir returns the project-level memory directory (with trailing separator).
// Kept for callers that only care about project-scoped state.
func (m *Manager) Dir() string {
	return m.memDir
}

// UserDir returns the user-level memory directory (with trailing separator).
func (m *Manager) UserDir() string {
	return m.userMemDir
}

// EntrypointPath returns the absolute path to the project-level MEMORY.md.
func (m *Manager) EntrypointPath() string {
	return filepath.Join(m.memDir, AutoMemEntrypointName)
}

// UserEntrypointPath returns the absolute path to the user-level MEMORY.md.
func (m *Manager) UserEntrypointPath() string {
	if m.userMemDir == "" {
		return ""
	}
	return filepath.Join(m.userMemDir, AutoMemEntrypointName)
}

// BuildSystemReminder returns the `# auto memory` section ready for inclusion
// in the system prompt. Ensures both directories exist so the agent can Write
// to either without first running mkdir.
func (m *Manager) BuildSystemReminder() string {
	if m.memDir == "" && m.userMemDir == "" {
		return ""
	}
	if m.userMemDir != "" {
		_ = EnsureMemoryDirExists(m.userMemDir)
	}
	if m.memDir != "" {
		_ = EnsureMemoryDirExists(m.memDir)
	}
	return BuildMemoryPrompt(autoMemDisplayName, m.userMemDir, m.memDir)
}

// MemoryFile describes one saved memory.
type MemoryFile struct {
	Path        string
	Name        string
	Description string
	Type        MemoryType
}

// GetMemories returns one-line summaries of every memory file in the
// memory directory. Used by the `/memory list` slash command. Order is
// stable (sorted by filename).
func (m *Manager) GetMemories() []string {
	files := m.LoadAll()
	out := make([]string, 0, len(files))
	for _, f := range files {
		typeTag := string(f.Type)
		if typeTag == "" {
			typeTag = "?"
		}
		desc := f.Description
		if desc == "" {
			desc = filepath.Base(f.Path)
		}
		out = append(out, fmt.Sprintf("[%s] %s — %s", typeTag, f.Name, desc))
	}
	return out
}

// LoadAll scans both the user-level and project-level memory directories for
// *.md files (excluding MEMORY.md) and returns parsed frontmatter for each.
// User-level files come first, then project-level.
func (m *Manager) LoadAll() []MemoryFile {
	out := loadDir(m.userMemDir)
	out = append(out, loadDir(m.memDir)...)
	return out
}

func loadDir(dir string) []MemoryFile {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var out []MemoryFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == AutoMemEntrypointName || !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		mf := parseFrontmatter(string(data))
		mf.Path = path
		if mf.Name == "" {
			mf.Name = strings.TrimSuffix(name, ".md")
		}
		out = append(out, mf)
	}
	return out
}

// Clear removes every *.md file (including MEMORY.md) in both memory
// directories. Used by the `/memory clear` slash command.
func (m *Manager) Clear() {
	clearDir(m.userMemDir)
	clearDir(m.memDir)
}

func clearDir(dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}

var frontmatterRe = regexp.MustCompile(`(?s)\A---\s*\n(.*?)\n---\s*\n`)

// parseFrontmatter extracts name/description/type from YAML-ish frontmatter.
// Only the three known fields are read; everything else (including unknown
// fields and full YAML semantics like quoting) is ignored. Files without
// frontmatter degrade gracefully — empty fields, body is the whole file.
func parseFrontmatter(content string) MemoryFile {
	var mf MemoryFile
	m := frontmatterRe.FindStringSubmatch(content)
	if m == nil {
		return mf
	}
	for line := range strings.SplitSeq(m[1], "\n") {
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)
		val = strings.Trim(val, `"'`)
		switch key {
		case "name":
			mf.Name = val
		case "description":
			mf.Description = val
		case "type":
			if t, ok := ParseMemoryType(val); ok {
				mf.Type = t
			}
		}
	}
	return mf
}
