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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseFrontmatterOnly does phase-1 loading: read just enough of the skill
// file to extract the SkillMeta, leave PromptBody empty. Cheap enough to run
// for hundreds of skills at startup.
//
// Accepts both layouts:
//   - <dir>/skill.yaml (+ optional prompt.md, ignored at phase 1)
//   - <dir>/SKILL.md with `---` YAML frontmatter
func parseFrontmatterOnly(dir string) (*Skill, error) {
	yamlPath := filepath.Join(dir, "skill.yaml")
	if data, err := os.ReadFile(yamlPath); err == nil {
		var meta SkillMeta
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parse skill.yaml: %w", err)
		}
		applyMetaDefaults(&meta, dir, "")
		return &Skill{
			Meta:        meta,
			SourceDir:   dir,
			IsDirectory: true,
			BodyLoaded:  false,
		}, nil
	}

	mdPath := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("no skill.yaml or SKILL.md: %w", err)
	}
	meta, _ := splitFrontmatter(string(data))
	applyMetaDefaults(&meta, dir, string(data))
	return &Skill{
		Meta:        meta,
		SourceDir:   dir,
		IsDirectory: true,
		BodyLoaded:  false,
	}, nil
}

// loadSkillBody reads the body for an already-frontmatter-parsed skill.
// Called by Catalog.GetFull each time the skill is invoked (hot reload).
// On any read/parse error, leaves the existing PromptBody untouched and
// returns the error so the caller can fall back to the cached version.
func loadSkillBody(skill *Skill) error {
	yamlPath := filepath.Join(skill.SourceDir, "skill.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		promptPath := filepath.Join(skill.SourceDir, "prompt.md")
		body, err := os.ReadFile(promptPath)
		if err != nil {
			return fmt.Errorf("read prompt.md: %w", err)
		}
		skill.PromptBody = string(body)
		skill.BodyLoaded = true
		return nil
	}

	mdPath := filepath.Join(skill.SourceDir, "SKILL.md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return fmt.Errorf("read SKILL.md: %w", err)
	}
	_, body := splitFrontmatter(string(data))
	skill.PromptBody = body
	skill.BodyLoaded = true
	return nil
}

// loadSkillFromBytes parses a skill from in-memory bytes (used for go:embed
// builtins where there's no on-disk directory to re-read).
func loadSkillFromBytes(name string, mdBytes []byte) (*Skill, error) {
	meta, body := splitFrontmatter(string(mdBytes))
	if meta.Name == "" {
		meta.Name = name
	}
	applyMetaDefaults(&meta, name, string(mdBytes))
	return &Skill{
		Meta:        meta,
		PromptBody:  body,
		SourceDir:   "", // embedded — no source dir
		IsDirectory: false,
		BodyLoaded:  true,
	}, nil
}

// splitFrontmatter separates the YAML frontmatter from the markdown body.
// Returns zero-value meta if no `---` frontmatter is present.
func splitFrontmatter(content string) (SkillMeta, string) {
	var meta SkillMeta
	body := content

	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			if err := yaml.Unmarshal([]byte(parts[1]), &meta); err == nil {
				body = strings.TrimSpace(parts[2])
			}
		}
	}
	return meta, body
}

// applyMetaDefaults fills in name/description fallbacks the way the legacy
// parseSkillMD did:
//   - missing name → derive from dir basename (lower + kebab)
//   - missing description → first non-blank, non-heading body line
//   - missing Mode → "inline"
//   - missing ForkContext → "none" (only relevant when Mode == "fork")
func applyMetaDefaults(meta *SkillMeta, dirOrName, body string) {
	if meta.Name == "" {
		base := filepath.Base(dirOrName)
		meta.Name = strings.ToLower(strings.ReplaceAll(base, " ", "-"))
	}
	if meta.Description == "" && body != "" {
		for line := range strings.SplitSeq(body, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "---") {
				meta.Description = line
				break
			}
		}
	}
	if meta.Mode == "" {
		if meta.Context == "fork" {
			meta.Mode = "fork"
		} else {
			meta.Mode = "inline"
		}
	}
	if meta.IsFork() && meta.ForkContext == "" {
		meta.ForkContext = "none"
	}
}
