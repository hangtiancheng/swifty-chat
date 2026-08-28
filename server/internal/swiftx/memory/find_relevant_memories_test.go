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
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFindRelevantMemoriesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	called := false
	selector := func(ctx context.Context, sys, user string) (string, error) {
		called = true
		return "", nil
	}
	got, err := FindRelevantMemories(context.Background(), "anything", "", dir, nil, nil, selector)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result for empty dir, got %d", len(got))
	}
	if called {
		t.Error("selector should not be called when dir is empty")
	}
}

func TestFindRelevantMemoriesNilSelector(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")
	got, _ := FindRelevantMemories(context.Background(), "q", "", dir, nil, nil, nil)
	if got != nil {
		t.Errorf("expected nil when selector is nil, got %v", got)
	}
}

func TestFindRelevantMemoriesPicksValidFilenames(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")
	writeMD(t, filepath.Join(dir, "b.md"), "---\ntype: feedback\n---\nbody")
	writeMD(t, filepath.Join(dir, "c.md"), "---\ntype: project\n---\nbody")

	selector := func(ctx context.Context, sys, user string) (string, error) {
		return `{"selected_memories": ["a.md", "c.md", "ghost.md"]}`, nil
	}

	got, err := FindRelevantMemories(context.Background(), "query", "", dir, nil, nil, selector)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 selected (ghost.md filtered), got %d", len(got))
	}
	paths := []string{got[0].Path, got[1].Path}
	if !contains(paths, filepath.Join(dir, "a.md")) || !contains(paths, filepath.Join(dir, "c.md")) {
		t.Errorf("returned paths missing expected files: %+v", paths)
	}
	for _, m := range got {
		if m.MtimeMs == 0 {
			t.Errorf("mtime not threaded through: %+v", m)
		}
	}
}

func TestFindRelevantMemoriesAlreadySurfaced(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")
	writeMD(t, filepath.Join(dir, "b.md"), "---\ntype: feedback\n---\nbody")

	surfaced := map[string]struct{}{
		filepath.Join(dir, "a.md"): {},
	}
	var sawManifest string
	selector := func(ctx context.Context, sys, user string) (string, error) {
		sawManifest = user
		return `{"selected_memories": ["b.md"]}`, nil
	}
	got, _ := FindRelevantMemories(context.Background(), "q", "", dir, nil, surfaced, selector)
	if len(got) != 1 || got[0].Path != filepath.Join(dir, "b.md") {
		t.Errorf("expected only b.md, got %+v", got)
	}
	if strings.Contains(sawManifest, "a.md") {
		t.Errorf("surfaced file leaked into selector manifest: %s", sawManifest)
	}
}

func TestFindRelevantMemoriesBadJSON(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")

	selector := func(ctx context.Context, sys, user string) (string, error) {
		return "this is not JSON at all", nil
	}
	got, err := FindRelevantMemories(context.Background(), "q", "", dir, nil, nil, selector)
	if err != nil {
		t.Errorf("bad JSON should not propagate error, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result on bad JSON, got %+v", got)
	}
}

func TestFindRelevantMemoriesJSONInMarkdownFence(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")

	selector := func(ctx context.Context, sys, user string) (string, error) {
		return "Sure! Here you go:\n```json\n{\"selected_memories\": [\"a.md\"]}\n```", nil
	}
	got, _ := FindRelevantMemories(context.Background(), "q", "", dir, nil, nil, selector)
	if len(got) != 1 {
		t.Errorf("expected 1 selected from markdown-wrapped JSON, got %+v", got)
	}
}

func TestFindRelevantMemoriesSelectorError(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")

	selector := func(ctx context.Context, sys, user string) (string, error) {
		return "", errors.New("network down")
	}
	got, err := FindRelevantMemories(context.Background(), "q", "", dir, nil, nil, selector)
	if err != nil {
		t.Errorf("selector error should be swallowed, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty on selector error, got %+v", got)
	}
}

func TestFindRelevantMemoriesIncludesRecentTools(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")

	var sawUser string
	selector := func(ctx context.Context, sys, user string) (string, error) {
		sawUser = user
		return `{"selected_memories": []}`, nil
	}
	_, _ = FindRelevantMemories(context.Background(), "q", "", dir, []string{"Bash", "Grep"}, nil, selector)
	if !strings.Contains(sawUser, "Recently used tools: Bash, Grep") {
		t.Errorf("recent tools not included in user message: %s", sawUser)
	}
}

func TestFindRelevantMemoriesContextCancel(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	selector := func(ctx context.Context, sys, user string) (string, error) {
		return "", ctx.Err()
	}
	got, err := FindRelevantMemories(ctx, "q", "", dir, nil, nil, selector)
	if err != nil {
		t.Errorf("cancellation should not propagate error, got: %v", err)
	}
	_ = got
}

func TestFindRelevantMemoriesSystemPromptCarriesRule(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, filepath.Join(dir, "a.md"), "---\ntype: user\n---\nbody")

	var sawSystem string
	selector := func(ctx context.Context, sys, user string) (string, error) {
		sawSystem = sys
		return `{"selected_memories": []}`, nil
	}
	_, _ = FindRelevantMemories(context.Background(), "q", "", dir, nil, nil, selector)
	for _, expect := range []string{
		"selecting memories",
		"up to 5",
		"valid JSON only",
		"selected_memories",
	} {
		if !strings.Contains(sawSystem, expect) {
			t.Errorf("system prompt missing %q\n--- prompt:\n%s", expect, sawSystem)
		}
	}
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}
