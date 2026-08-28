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

package tools

import (
	"os"
	"path/filepath"
	"sync"
)

// FileStateCache tracks which files have been read and their modification
// times, enforcing a "read-before-edit" discipline to prevent blind overwrites.
type FileStateCache struct {
	mu      sync.Mutex
	entries map[string]int64 // path → mtime (UnixMilli)
}

func NewFileStateCache() *FileStateCache {
	return &FileStateCache{
		entries: make(map[string]int64),
	}
}

// Record stores the file mtime after a successful read.
func (c *FileStateCache) Record(filePath string, mtime int64) {
	abs := normalizePath(filePath)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[abs] = mtime
}

// Check verifies that a file has been read and hasn't been modified since.
// Returns (true, "") if OK, or (false, errorMessage) if the edit should be
// blocked.
func (c *FileStateCache) Check(filePath string) (bool, string) {
	abs := normalizePath(filePath)
	c.mu.Lock()
	cachedMtime, exists := c.entries[abs]
	c.mu.Unlock()

	if !exists {
		return false, "Error: file has not been read yet. Read it first before editing."
	}

	info, err := os.Stat(abs)
	if err != nil {
		// File might have been deleted — let the caller handle that.
		return true, ""
	}
	currentMtime := info.ModTime().UnixMilli()
	if currentMtime > cachedMtime {
		return false, "Error: file has been modified since last read. Read it again before editing."
	}

	return true, ""
}

// Update refreshes the cache entry after a successful edit or write.
func (c *FileStateCache) Update(filePath string) {
	abs := normalizePath(filePath)
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[abs] = info.ModTime().UnixMilli()
}

func normalizePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
