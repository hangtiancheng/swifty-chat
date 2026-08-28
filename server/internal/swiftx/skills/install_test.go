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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillURLShapes(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want SkillSource
	}{
		{
			name: "skills.sh canonical",
			url:  "https://www.skills.sh/anthropics/skills/frontend-design",
			want: SkillSource{
				Owner:   "anthropics",
				Repo:    "skills",
				Ref:     "main",
				Subpath: "skills/frontend-design",
				Name:    "frontend-design",
			},
		},
		{
			name: "github tree URL",
			url:  "https://github.com/anthropics/skills/tree/v1.2/skills/pdf",
			want: SkillSource{
				Owner:   "anthropics",
				Repo:    "skills",
				Ref:     "v1.2",
				Subpath: "skills/pdf",
				Name:    "pdf",
			},
		},
		{
			name: "raw SKILL.md URL",
			url:  "https://raw.githubusercontent.com/foo/bar/main/skills/baz/SKILL.md",
			want: SkillSource{
				Owner:   "foo",
				Repo:    "bar",
				Ref:     "main",
				Subpath: "skills/baz",
				Name:    "baz",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSkillURL(tc.url)
			if err != nil {
				t.Fatalf("ParseSkillURL(%q): %v", tc.url, err)
			}
			if got.Owner != tc.want.Owner || got.Repo != tc.want.Repo || got.Ref != tc.want.Ref ||
				got.Subpath != tc.want.Subpath || got.Name != tc.want.Name {
				t.Errorf("got %+v, want %+v", *got, tc.want)
			}
		})
	}
}

func TestParseSkillURLRejects(t *testing.T) {
	bad := []string{
		"ftp://www.skills.sh/a/b/c",
		"https://example.com/some/path",
		"https://www.skills.sh/just-one-segment",
		"https://github.com/owner/repo", // no /tree/<ref>
	}
	for _, u := range bad {
		if _, err := ParseSkillURL(u); err == nil {
			t.Errorf("expected ParseSkillURL(%q) to fail", u)
		}
	}
}

func TestValidateSkillName(t *testing.T) {
	ok := []string{"frontend-design", "pdf", "backend_interview", "x1y2"}
	for _, n := range ok {
		if err := validateSkillName(n); err != nil {
			t.Errorf("validateSkillName(%q): %v", n, err)
		}
	}
	bad := []string{"", ".hidden", "Has-Caps", "with space", "with/slash", "with..dots"}
	for _, n := range bad {
		if err := validateSkillName(n); err == nil {
			t.Errorf("expected reject for %q", n)
		}
	}
}

// fakeGitHub stubs the Contents API + raw file endpoints so we can drive
// installWith deterministically. Layout:
//
//	skills/sample/SKILL.md  -> "---\nname: sample\n..."
//	skills/sample/references/note.md
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	skillMD := "---\nname: sample\ndescription: sample skill\nmode: inline\n---\n\nSample body."
	noteMD := "extra reference"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/foo/bar/contents/skills/sample":
			entries := []contentEntry{
				{Name: "SKILL.md", Path: "skills/sample/SKILL.md", Type: "file", Size: len(skillMD),
					Encoding: "base64", Content: base64.StdEncoding.EncodeToString([]byte(skillMD))},
				{Name: "references", Path: "skills/sample/references", Type: "dir"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(entries)
		case "/repos/foo/bar/contents/skills/sample/references":
			entries := []contentEntry{
				{Name: "note.md", Path: "skills/sample/references/note.md", Type: "file", Size: len(noteMD),
					Encoding: "base64", Content: base64.StdEncoding.EncodeToString([]byte(noteMD))},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(entries)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "unmocked path %s", r.URL.Path)
		}
	}))
	return srv
}

func TestInstallEndToEnd(t *testing.T) {
	srv := fakeGitHub(t)
	defer srv.Close()

	src := &SkillSource{Owner: "foo", Repo: "bar", Ref: "main", Subpath: "skills/sample", Name: "sample"}
	root := t.TempDir()
	f := &fetcher{client: srv.Client(), apiBase: srv.URL}

	rep, err := installWith(f, src, root)
	if err != nil {
		t.Fatalf("installWith: %v", err)
	}
	if rep.SkillName != "sample" || rep.FileCount != 2 {
		t.Errorf("unexpected report: %+v", rep)
	}
	skillMD := filepath.Join(root, "sample", "SKILL.md")
	data, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "Sample body.") {
		t.Errorf("SKILL.md body mismatch: %q", string(data))
	}
	noteMD := filepath.Join(root, "sample", "references", "note.md")
	if _, err := os.Stat(noteMD); err != nil {
		t.Errorf("nested reference not installed: %v", err)
	}
}

func TestInstallRejectsTreeWithoutSkillMD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a tree containing only an unrelated file.
		entries := []contentEntry{
			{Name: "README.md", Path: "x/y/README.md", Type: "file", Size: 4,
				Encoding: "base64", Content: base64.StdEncoding.EncodeToString([]byte("hi\n"))},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	f := &fetcher{client: srv.Client(), apiBase: srv.URL}
	src := &SkillSource{Owner: "x", Repo: "y", Ref: "main", Subpath: "x/y", Name: "y"}
	if _, err := installWith(f, src, t.TempDir()); err == nil {
		t.Fatal("expected install to reject tree without SKILL.md")
	}
}
