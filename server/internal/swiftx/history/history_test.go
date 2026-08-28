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

package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	entries := Load(dir)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestAppendAndLoad(t *testing.T) {
	dir := t.TempDir()

	Append(dir, "hello")
	Append(dir, "world")

	entries := Load(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0] != "hello" || entries[1] != "world" {
		t.Fatalf("unexpected entries: %v", entries)
	}
}

func TestDedup(t *testing.T) {
	dir := t.TempDir()

	Append(dir, "same")
	Append(dir, "same")

	entries := Load(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after dedup, got %d", len(entries))
	}
}

func TestTrim(t *testing.T) {
	dir := t.TempDir()

	for i := range 210 {
		Append(dir, "entry"+string(rune('A'+i%26))+string(rune('0'+i/26)))
	}

	entries := Load(dir)
	if len(entries) > maxEntries {
		t.Fatalf("expected <= %d entries, got %d", maxEntries, len(entries))
	}
}

func TestFileCreated(t *testing.T) {
	dir := t.TempDir()

	Append(dir, "test")

	path := filepath.Join(dir, ".swiftx", "prompt_history.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("history file was not created")
	}
}
