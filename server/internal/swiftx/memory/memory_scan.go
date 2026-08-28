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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryHeader is one scanned memory file's metadata.
type MemoryHeader struct {
	Filename    string     // path relative to memoryDir
	FilePath    string     // absolute path
	Scope       string     // "user" or "project"; empty for callers that don't care
	MtimeMs     int64      // modification time, ms since epoch
	Description string     // frontmatter description; "" if absent
	Type        MemoryType // frontmatter type; "" if unrecognized
}

// MaxMemoryFiles caps the number of memories surfaced to the model.
// FrontmatterMaxLines caps how much of each file we read for header parsing.
const (
	MaxMemoryFiles      = 200
	FrontmatterMaxLines = 30
)

// ScanMemoryFiles scans a memory directory for .md files, reads their
// frontmatter, and returns a header list sorted newest-first (capped at
// MaxMemoryFiles). Shared by findRelevantMemories (query-time recall) and
// extractMemories (pre-injects the listing so the extraction agent doesn't
// spend a turn on `ls`).
//
// Single-pass: each file's mtime is read alongside its content so we
// read-then-sort rather than stat-sort-read. Per-file errors are silently
// dropped (a file that won't open shouldn't tank the whole scan).
func ScanMemoryFiles(ctx context.Context, memoryDir string, scope string) ([]MemoryHeader, error) {
	var mdFiles []string
	walkErr := filepath.WalkDir(memoryDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".md") || name == AutoMemEntrypointName {
			return nil
		}
		mdFiles = append(mdFiles, path)
		return nil
	})
	if walkErr != nil {
		return nil, nil
	}

	results := make([]MemoryHeader, 0, len(mdFiles))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, filePath := range mdFiles {
		if err := ctx.Err(); err != nil {
			break
		}
		wg.Add(1)
		go func(fp string) {
			defer wg.Done()
			hdr, ok := readMemoryHeader(fp, memoryDir)
			if !ok {
				return
			}
			hdr.Scope = scope
			mu.Lock()
			results = append(results, hdr)
			mu.Unlock()
		}(filePath)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].MtimeMs > results[j].MtimeMs
	})
	if len(results) > MaxMemoryFiles {
		results = results[:MaxMemoryFiles]
	}
	return results, nil
}

func readMemoryHeader(filePath, memoryDir string) (MemoryHeader, bool) {
	info, err := os.Stat(filePath)
	if err != nil {
		return MemoryHeader{}, false
	}
	f, err := os.Open(filePath)
	if err != nil {
		return MemoryHeader{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var sb strings.Builder
	for i := 0; i < FrontmatterMaxLines && scanner.Scan(); i++ {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}

	mf := parseFrontmatter(sb.String())
	rel, err := filepath.Rel(memoryDir, filePath)
	if err != nil {
		rel = filepath.Base(filePath)
	}
	return MemoryHeader{
		Filename:    rel,
		FilePath:    filePath,
		MtimeMs:     info.ModTime().UnixMilli(),
		Description: mf.Description,
		Type:        mf.Type,
	}, true
}

// FormatMemoryManifest formats memory headers as a text manifest: one
// line per file with [type] filename (timestamp): description. Used by
// both the recall selector prompt and the extraction-agent prompt.
func FormatMemoryManifest(memories []MemoryHeader) string {
	if len(memories) == 0 {
		return ""
	}
	var b strings.Builder
	for i, m := range memories {
		if i > 0 {
			b.WriteByte('\n')
		}
		var tag string
		if m.Type != "" {
			tag = fmt.Sprintf("[%s] ", m.Type)
		}
		var scope string
		if m.Scope != "" {
			scope = fmt.Sprintf("[%s-scope] ", m.Scope)
		}
		ts := time.UnixMilli(m.MtimeMs).UTC().Format("2006-01-02T15:04:05.000Z")
		path := m.FilePath
		if path == "" {
			path = m.Filename
		}
		if m.Description != "" {
			fmt.Fprintf(&b, "- %s%s%s (%s): %s", scope, tag, path, ts, m.Description)
		} else {
			fmt.Fprintf(&b, "- %s%s%s (%s)", scope, tag, path, ts)
		}
	}
	return b.String()
}
