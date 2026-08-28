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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxIncludeDepth is the maximum nesting depth for @include directives.
const MaxIncludeDepth = 5

// InstructionSource is one loaded instruction file.
type InstructionSource struct {
	Path    string
	Content string
}

// LoadInstructions discovers and concatenates project & user instruction files.
//
// Discovery order (each later layer is appended later, so model attention prioritises it):
//  1. User global: ~/.swiftx/SWIFTX.md, ~/.swiftx/AGENTS.md
//  2. Project: walk from git root down to workDir, picking up SWIFTX.md,
//     AGENTS.md and .swiftx/SWIFTX.md in each directory (so the file
//     closest to cwd wins)
//  3. workDir/SWIFTX.local.md (private local override)
//
// @-include directives:
//   - @./relative/path, @~/home/path, or @/absolute/path
//   - Resolved relative to the including file's directory
//   - Skipped inside fenced code blocks
//   - Cycle-safe (same absolute path is never included twice)
func LoadInstructions(workDir string) string {
	sources := DiscoverInstructions(workDir)
	if len(sources) == 0 {
		return ""
	}
	var parts []string
	for _, s := range sources {
		label := s.Path
		if rel, err := filepath.Rel(workDir, s.Path); err == nil && !strings.HasPrefix(rel, "..") {
			label = rel
		}
		parts = append(parts, fmt.Sprintf("Contents of %s:\n\n%s", label, strings.TrimRight(s.Content, "\n")))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// DiscoverInstructions returns the loaded source files in priority order
// (lowest priority first). Used by LoadInstructions and exposed for tests.
func DiscoverInstructions(workDir string) []InstructionSource {
	var sources []InstructionSource
	seen := map[string]bool{}

	// Determine project root for @include path boundary checks
	absWorkDir, _ := filepath.Abs(workDir)
	projectRoot := findGitRoot(absWorkDir)
	if projectRoot == "" {
		projectRoot = absWorkDir
	}

	if home, err := os.UserHomeDir(); err == nil {
		add(&sources, seen, filepath.Join(home, ".swiftx", "SWIFTX.md"), projectRoot)
		add(&sources, seen, filepath.Join(home, ".swiftx", "AGENTS.md"), projectRoot)
	}
	for _, dir := range projectInstructionDirs(workDir) {
		add(&sources, seen, filepath.Join(dir, "SWIFTX.md"), projectRoot)
		add(&sources, seen, filepath.Join(dir, "AGENTS.md"), projectRoot)
		// Same-named file under .swiftx/: for projects that gitignore their instructions
		add(&sources, seen, filepath.Join(dir, ".swiftx", "SWIFTX.md"), projectRoot)
	}
	add(&sources, seen, filepath.Join(workDir, "SWIFTX.local.md"), projectRoot)
	return sources
}

func add(out *[]InstructionSource, seen map[string]bool, path, projectRoot string) {
	abs, err := filepath.Abs(path)
	if err != nil || seen[abs] {
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	seen[abs] = true
	content := expandIncludes(string(data), filepath.Dir(abs), projectRoot, seen, 0)
	*out = append(*out, InstructionSource{Path: abs, Content: content})
}

// expandIncludes expands @include directives. projectRoot is used for boundary checks
// to prevent escaping to arbitrary locations outside the project directory via ../.
func expandIncludes(content, baseDir, projectRoot string, seen map[string]bool, depth int) string {
	if depth > MaxIncludeDepth {
		return content
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var out strings.Builder
	inCode := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		if !inCode {
			if path := parseInclude(trimmed); path != "" {
				resolved := resolveInclude(path, baseDir)
				if abs, err := filepath.Abs(resolved); err == nil && !seen[abs] {
					// @include path boundary check: do not allow escaping outside project and user home
					if !isIncludeAllowed(abs, projectRoot) {
						out.WriteString("<!-- @include skipped: path outside project -->\n")
						continue
					}
					if data, err := os.ReadFile(abs); err == nil {
						seen[abs] = true
						fmt.Fprintf(&out, "<!-- included from %s -->\n", path)
						out.WriteString(expandIncludes(string(data), filepath.Dir(abs), projectRoot, seen, depth+1))
						out.WriteByte('\n')
						continue
					}
				}
				// Fall through if include can't be resolved/read; emit the
				// original line so the user notices.
			}
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// isIncludeAllowed checks whether the resolved absolute path of an @include is within the allowed scope.
// Allowed scope: the project directory (projectRoot) and its subdirectories, as well as .swiftx/ under the user's home directory.
// This prevents arbitrary path traversal via @../../etc/passwd and similar patterns.
func isIncludeAllowed(absPath, projectRoot string) bool {
	// Paths within the project directory are always allowed
	if projectRoot != "" && strings.HasPrefix(absPath, projectRoot+string(filepath.Separator)) {
		return true
	}
	if absPath == projectRoot {
		return true
	}
	// .swiftx/ under user home is also allowed (global instruction file includes)
	if home, err := os.UserHomeDir(); err == nil {
		swiftxDir := filepath.Join(home, ".swiftx")
		if strings.HasPrefix(absPath, swiftxDir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// parseInclude returns the include path for a line of the form
// "@./path", "@~/path", or "@/abs/path", else "". Other @-tokens (e.g.
// @username) are ignored to avoid false positives.
func parseInclude(trimmed string) string {
	if !strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "@@") {
		return ""
	}
	rest := strings.TrimPrefix(trimmed, "@")
	if rest == "" {
		return ""
	}
	if strings.ContainsAny(rest, " \t") {
		return ""
	}
	switch {
	case strings.HasPrefix(rest, "./"), strings.HasPrefix(rest, "../"),
		strings.HasPrefix(rest, "~/"), strings.HasPrefix(rest, "/"):
		return rest
	}
	return ""
}

func resolveInclude(p, baseDir string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}

// projectInstructionDirs returns directories from git root down to workDir.
// If workDir is not inside a git repo, only [workDir] is returned.
func projectInstructionDirs(workDir string) []string {
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return []string{workDir}
	}
	root := findGitRoot(abs)
	if root == "" {
		return []string{abs}
	}
	var dirs []string
	cur := abs
	for {
		dirs = append([]string{cur}, dirs...)
		if cur == root {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return dirs
}

func findGitRoot(start string) string {
	cur := start
	for {
		if info, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			_ = info
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}
