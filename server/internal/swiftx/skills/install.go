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

package skills

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SkillSource describes where to pull a skill from. Always normalises down
// to a GitHub Contents API path because skills.sh is just a registry that
// points at GitHub trees.
type SkillSource struct {
	Owner    string
	Repo     string
	Ref      string // branch or tag; "main" when unspecified
	Subpath  string // path within the repo to the skill dir (no trailing /)
	Name     string // skill name (== last segment of Subpath)
	Original string // user-supplied URL, for error messages
}

// ParseSkillURL accepts three URL shapes:
//
//  1. https://www.skills.sh/<owner>/<repo>/<skill-name>
//     — assumes the skill lives at "skills/<skill-name>" in the repo
//     (anthropics/skills convention)
//  2. https://github.com/<owner>/<repo>/tree/<ref>/<subpath>
//     — direct subtree URL; last segment is the skill name
//  3. https://raw.githubusercontent.com/<owner>/<repo>/<ref>/<subpath>/SKILL.md
//     — raw file URL; treat parent dir as the skill subpath
//
// Returns a fully-resolved SkillSource, or an error if the URL doesn't
// match any of the three shapes.
func ParseSkillURL(raw string) (*SkillSource, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http(s) URLs are supported")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")

	switch u.Host {
	case "www.skills.sh", "skills.sh":
		if len(parts) < 3 {
			return nil, fmt.Errorf("skills.sh URL must be /<owner>/<repo>/<skill-name>")
		}
		return &SkillSource{
			Owner:    parts[0],
			Repo:     parts[1],
			Ref:      "main",
			Subpath:  "skills/" + strings.Join(parts[2:], "/"),
			Name:     parts[len(parts)-1],
			Original: raw,
		}, nil

	case "github.com":
		// Expected: /<owner>/<repo>/tree/<ref>/<...subpath>
		if len(parts) < 5 || parts[2] != "tree" {
			return nil, fmt.Errorf("github.com URL must be /<owner>/<repo>/tree/<ref>/<subpath>")
		}
		sub := strings.Join(parts[4:], "/")
		return &SkillSource{
			Owner:    parts[0],
			Repo:     parts[1],
			Ref:      parts[3],
			Subpath:  sub,
			Name:     parts[len(parts)-1],
			Original: raw,
		}, nil

	case "raw.githubusercontent.com":
		// Expected: /<owner>/<repo>/<ref>/<...subpath>/SKILL.md
		if len(parts) < 4 {
			return nil, fmt.Errorf("raw.githubusercontent.com URL too short")
		}
		// Strip trailing filename so Subpath ends at the skill dir.
		subParts := parts[3:]
		if last := subParts[len(subParts)-1]; strings.Contains(last, ".") {
			subParts = subParts[:len(subParts)-1]
		}
		if len(subParts) == 0 {
			return nil, fmt.Errorf("raw URL missing skill subpath")
		}
		return &SkillSource{
			Owner:    parts[0],
			Repo:     parts[1],
			Ref:      parts[2],
			Subpath:  strings.Join(subParts, "/"),
			Name:     subParts[len(subParts)-1],
			Original: raw,
		}, nil
	}
	return nil, fmt.Errorf("unsupported host %q (try skills.sh or github.com)", u.Host)
}

// contentEntry holds the subset of the GitHub Contents API response we
// care about. Type is "file" | "dir" | "symlink" | "submodule"; we only
// follow files and dirs.
type contentEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
	Content     string `json:"content"`
	Encoding    string `json:"encoding"`
	Size        int    `json:"size"`
}

// installLimits caps how much data we'll pull from a remote source.
// Single-skill installs are tiny (SKILL.md + maybe a few reference files);
// anything bigger probably means we got a wrong URL or a bad actor.
const (
	maxFileSize       = 1 << 20 // 1 MiB per file
	maxTotalSize      = 8 << 20 // 8 MiB per skill
	maxFileCount      = 64
	maxRecursionDepth = 4
	httpTimeout       = 30 * time.Second
)

// fetcher centralises HTTP calls so tests can swap the underlying client
// and so we apply consistent headers and timeouts.
type fetcher struct {
	client  *http.Client
	apiBase string // "https://api.github.com" — overridable for tests
}

func newFetcher() *fetcher {
	return &fetcher{
		client:  &http.Client{Timeout: httpTimeout},
		apiBase: "https://api.github.com",
	}
}

func (f *fetcher) listContents(src *SkillSource, subpath string) ([]contentEntry, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s",
		f.apiBase, src.Owner, src.Repo, subpath, url.QueryEscape(src.Ref))
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "swiftx-install-skill")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contents API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		// Rate-limited; surface the body so user sees the GitHub error msg.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github API forbidden (rate-limited?): %s", strings.TrimSpace(string(body)))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d for %s", resp.StatusCode, endpoint)
	}

	// The endpoint returns an array for directories and a single object for
	// files. Decode into a generic value and dispatch.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFileSize))
	if err != nil {
		return nil, fmt.Errorf("read contents response: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("github returned empty body")
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var entries []contentEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("parse dir listing: %w", err)
		}
		return entries, nil
	}
	var single contentEntry
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, fmt.Errorf("parse file metadata: %w", err)
	}
	return []contentEntry{single}, nil
}

// fetchBlob downloads a single file's bytes. Prefers the inlined base64
// `content` field when present (cheaper than a second round-trip) and
// falls back to download_url for binaries / files >1MB.
func (f *fetcher) fetchBlob(e contentEntry) ([]byte, error) {
	if e.Size > maxFileSize {
		return nil, fmt.Errorf("file %s too large: %d bytes (max %d)", e.Path, e.Size, maxFileSize)
	}
	if e.Encoding == "base64" && e.Content != "" {
		clean := strings.ReplaceAll(e.Content, "\n", "")
		out, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("decode base64 for %s: %w", e.Path, err)
		}
		return out, nil
	}
	if e.DownloadURL == "" {
		return nil, fmt.Errorf("no download_url for %s", e.Path)
	}
	req, _ := http.NewRequest("GET", e.DownloadURL, nil)
	req.Header.Set("User-Agent", "swiftx-install-skill")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", e.DownloadURL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFileSize))
}

// InstallReport summarises what an Install call did, returned by the tool
// for display and consumed by tests.
type InstallReport struct {
	SkillName  string
	TargetDir  string
	FileCount  int
	TotalBytes int64
}

// Install pulls the skill at src into installRoot/<src.Name>/. installRoot
// is expected to be the user-global skills tier (~/.swiftx/skills/) so
// installs are reused across projects.
//
// Writes are atomic at the directory level: we first stage into a sibling
// tempdir, then rename into place. Partial failures leave installRoot
// unchanged.
func Install(src *SkillSource, installRoot string) (*InstallReport, error) {
	return installWith(newFetcher(), src, installRoot)
}

func installWith(f *fetcher, src *SkillSource, installRoot string) (*InstallReport, error) {
	if src == nil {
		return nil, fmt.Errorf("nil source")
	}
	if err := validateSkillName(src.Name); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create install root: %w", err)
	}
	staging, err := os.MkdirTemp(installRoot, ".install-"+src.Name+"-*")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	cleanupStaging := func() { _ = os.RemoveAll(staging) }

	report := &InstallReport{SkillName: src.Name}
	if err := walkAndDownload(f, src, src.Subpath, staging, report, 0); err != nil {
		cleanupStaging()
		return nil, err
	}
	if !hasSkillManifest(staging) {
		cleanupStaging()
		return nil, fmt.Errorf("downloaded tree missing SKILL.md or skill.yaml — not a skill?")
	}

	final := filepath.Join(installRoot, src.Name)
	if _, err := os.Stat(final); err == nil {
		// Overwrite an existing install by removing it first. The user
		// explicitly asked to install — assume they want the latest.
		if err := os.RemoveAll(final); err != nil {
			cleanupStaging()
			return nil, fmt.Errorf("remove old install: %w", err)
		}
	}
	if err := os.Rename(staging, final); err != nil {
		cleanupStaging()
		return nil, fmt.Errorf("promote staging dir: %w", err)
	}
	report.TargetDir = final
	return report, nil
}

// walkAndDownload reproduces the on-GitHub directory tree under localDir,
// counting files + bytes against the install limits.
func walkAndDownload(f *fetcher, src *SkillSource, subpath, localDir string, report *InstallReport, depth int) error {
	if depth > maxRecursionDepth {
		return fmt.Errorf("install tree too deep (>%d levels)", maxRecursionDepth)
	}
	entries, err := f.listContents(src, subpath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if report.FileCount >= maxFileCount {
			return fmt.Errorf("install file count limit (%d) reached", maxFileCount)
		}
		// `Name` is the leaf — guard against path traversal even though
		// the GitHub API won't ever emit "../" itself.
		if strings.Contains(e.Name, "..") || strings.ContainsAny(e.Name, "/\\") {
			return fmt.Errorf("suspicious entry name: %q", e.Name)
		}
		target := filepath.Join(localDir, e.Name)
		switch e.Type {
		case "file":
			data, err := f.fetchBlob(e)
			if err != nil {
				return err
			}
			if int64(report.TotalBytes)+int64(len(data)) > maxTotalSize {
				return fmt.Errorf("install total size limit (%d bytes) reached", maxTotalSize)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", target, err)
			}
			report.FileCount++
			report.TotalBytes += int64(len(data))
		case "dir":
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			if err := walkAndDownload(f, src, e.Path, target, report, depth+1); err != nil {
				return err
			}
		default:
			// Skip symlinks / submodules silently — they shouldn't appear
			// in a well-formed skill.
		}
	}
	return nil
}

// hasSkillManifest verifies the staged tree contains a SKILL.md or
// skill.yaml at its root. Pre-flight guard against a "URL pointed at the
// wrong subdir" mistake.
func hasSkillManifest(dir string) bool {
	for _, name := range []string{"SKILL.md", "skill.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// validateSkillName allows kebab-case and snake_case; disallows path
// traversal, leading dots, and anything that'd surprise a shell.
func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("empty skill name")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("skill name cannot start with '.'")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("skill name %q contains invalid char %q (use a-z 0-9 - _)", name, r)
		}
	}
	return nil
}

// UserSkillsRoot returns ~/.swiftx/skills, creating the parent if needed
// so callers don't have to repeat the dance.
func UserSkillsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	root := filepath.Join(home, ".swiftx", "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}
