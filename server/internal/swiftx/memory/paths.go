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
	"os"
	"path/filepath"
	"strings"
)

// AutoMemEntrypointName is the filename of the per-project memory index.
const AutoMemEntrypointName = "MEMORY.md"

// GetAutoMemPath returns the auto-memory directory path for the given
// project root. Shape: <projectRoot>/.swiftx/memory/
//
// The trailing separator is preserved so prefix-based path matching (e.g.,
// sandbox `HasPrefix` checks) work correctly without falsely matching
// `…/memoryxyz`.
//
// Swiftx colocates memory with other project-local state under .swiftx/
// so records show up in the IDE and editors can open them directly.
//
// Resolution order:
//  1. SWIFTX_REMOTE_MEMORY_DIR env var — used as-is (escape hatch for
//     CI/container scenarios where memory should live elsewhere)
//  2. <projectRoot>/.swiftx/memory
func GetAutoMemPath(projectRoot string) string {
	if override := os.Getenv("SWIFTX_REMOTE_MEMORY_DIR"); override != "" {
		return strings.TrimRight(override, string(filepath.Separator)) + string(filepath.Separator)
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		abs = projectRoot
	}
	return filepath.Join(abs, ".swiftx", "memory") + string(filepath.Separator)
}

// GetAutoMemEntrypoint returns the path to the MEMORY.md inside the
// auto-memory directory.
func GetAutoMemEntrypoint(projectRoot string) string {
	dir := GetAutoMemPath(projectRoot)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, AutoMemEntrypointName)
}

// IsAutoMemPath checks if an absolute path is within EITHER the project-level
// or user-level auto-memory directory. Used by the path sandbox to allow
// Writes into either memory dir.
func IsAutoMemPath(absolutePath, projectRoot string) bool {
	abs := filepath.Clean(absolutePath)
	if dir := GetAutoMemPath(projectRoot); dir != "" {
		if strings.HasPrefix(abs+string(filepath.Separator), dir) {
			return true
		}
	}
	if dir := GetUserAutoMemPath(); dir != "" {
		if strings.HasPrefix(abs+string(filepath.Separator), dir) {
			return true
		}
	}
	return false
}

// GetUserAutoMemPath returns the user-level auto-memory directory:
// ~/.swiftx/memory/. Used for type=user / type=feedback memories that
// follow the human across projects (e.g. coding preferences). Returns ""
// if the home directory cannot be resolved.
//
// Trailing separator is preserved for prefix-based path matching.
func GetUserAutoMemPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".swiftx", "memory") + string(filepath.Separator)
}

// GetUserAutoMemEntrypoint returns the path to ~/.swiftx/memory/MEMORY.md.
func GetUserAutoMemEntrypoint() string {
	dir := GetUserAutoMemPath()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, AutoMemEntrypointName)
}

// IsUserAutoMemPath checks if an absolute path is within the user-level
// memory dir. Used in places where we need to distinguish user-scope from
// project-scope (sandbox already accepts both; this is for routing).
func IsUserAutoMemPath(absolutePath string) bool {
	dir := GetUserAutoMemPath()
	if dir == "" {
		return false
	}
	abs := filepath.Clean(absolutePath)
	return strings.HasPrefix(abs+string(filepath.Separator), dir)
}
