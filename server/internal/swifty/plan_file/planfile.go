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

package plan_file

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const PlansDir = ".swifty/plans"

// planPaths caches the active plan file per workDir. The chat server hosts
// many sessions in one process, so a single global path would leak session
// A's plan into session B.
var planPaths = struct {
	sync.Mutex
	m map[string]string
}{m: make(map[string]string)}

func plansDir(workDir string) string {
	return filepath.Join(workDir, PlansDir)
}

func generateSlug() string {
	adjectives := []string{
		"bright", "calm", "bold", "swift", "quiet",
		"vivid", "clear", "keen", "warm", "cool",
		"sharp", "light", "deep", "pure", "soft",
	}
	nouns := []string{
		"plan", "draft", "design", "sketch", "blueprint",
		"outline", "strategy", "approach", "scheme", "map",
		"vision", "path", "route", "guide", "frame",
	}
	now := time.Now()
	ai := int(now.UnixNano()/1000) % len(adjectives)
	ni := int(now.UnixNano()/100) % len(nouns)
	return fmt.Sprintf("%s-%s-%s", adjectives[ai], nouns[ni], now.Format("0102-1504"))
}

func GetOrCreatePlanPath(workDir string) string {
	planPaths.Lock()
	defer planPaths.Unlock()
	if path, ok := planPaths.m[workDir]; ok {
		return path
	}
	dir := plansDir(workDir)
	os.MkdirAll(dir, 0o755)
	slug := generateSlug()
	path := filepath.Join(dir, slug+".md")
	planPaths.m[workDir] = path
	return path
}

func PlanExists(workDir string) bool {
	planPaths.Lock()
	path, ok := planPaths.m[workDir]
	planPaths.Unlock()
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
