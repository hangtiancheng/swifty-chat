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

package prompt

import (
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Section struct {
	Name     string
	Priority int
	Content  string
}

type EnvironmentContext struct {
	WorkDir   string
	OS        string
	Arch      string
	Shell     string
	IsGitRepo bool
	GitBranch string
	Model     string
	Date      string
}

type Builder struct {
	sections []Section
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) Add(s Section) *Builder {
	b.sections = append(b.sections, s)
	return b
}

func (b *Builder) Build() string {
	sort.Slice(b.sections, func(i, j int) bool {
		return b.sections[i].Priority < b.sections[j].Priority
	})

	var parts []string
	for _, s := range b.sections {
		content := strings.TrimSpace(s.Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func DetectEnvironment(workDir string) EnvironmentContext {
	env := EnvironmentContext{
		WorkDir: workDir,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Shell:   os.Getenv("SHELL"),
		Date:    time.Now().Format("2006-01-02"),
	}

	if env.Shell == "" {
		env.Shell = "bash"
	}

	if out, err := exec.Command("git", "-C", workDir, "rev-parse", "--is-inside-work-tree").Output(); err == nil && strings.TrimSpace(string(out)) == "true" {
		env.IsGitRepo = true
		if branch, err := exec.Command("git", "-C", workDir, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			env.GitBranch = strings.TrimSpace(string(branch))
		}
	}

	return env
}

func BuildSystemPrompt(env EnvironmentContext) string {
	b := NewBuilder()

	b.Add(IdentitySection())
	b.Add(SystemSection())
	b.Add(DoingTasksSection())
	b.Add(ExecutingActionsSection())
	b.Add(UsingToolsSection())
	b.Add(ToneStyleSection())
	b.Add(OutputEfficiencySection())
	b.Add(EnvironmentSection(env))
	return b.Build()
}
