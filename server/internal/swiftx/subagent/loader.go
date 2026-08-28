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

package subagent

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AgentLoader struct {
	workDir string
	agents  map[string]*AgentDefinition

	// FailedFiles records definition files that failed to parse during the most recent LoadAll. Each
	// entry is "<path>: <reason>".
	FailedFiles []string

	// ErrorWriter receives one-line warnings for parse failures. Defaults to os.Stderr; tests override
	// it to capture output.
	ErrorWriter io.Writer
}

func NewAgentLoader(workDir string) *AgentLoader {
	return &AgentLoader{
		workDir:     workDir,
		agents:      make(map[string]*AgentDefinition),
		ErrorWriter: os.Stderr,
	}
}

// getBuiltinSpecs returns the built-in agent definitions. The verification
// agent is off by default; it is gated by the SWIFTX_VERIFICATION_AGENT
// environment variable and only added when enabled.
func getBuiltinSpecs() map[string]SubAgentSpec {
	result := make(map[string]SubAgentSpec, len(BuiltinSpecs)+1)
	maps.Copy(result, BuiltinSpecs)
	if os.Getenv("SWIFTX_VERIFICATION_AGENT") == "true" {
		result[VerificationAgentType] = verificationSpec
	}
	return result
}

func (l *AgentLoader) LoadAll() error {
	l.FailedFiles = l.FailedFiles[:0]
	for name, spec := range getBuiltinSpecs() {
		l.agents[name] = &AgentDefinition{
			AgentType:       spec.Name,
			WhenToUse:       spec.Description,
			DisallowedTools: spec.DisallowedTools,
			Model:           spec.Model,
			MaxTurns:        spec.MaxTurns,
			SystemPrompt:    spec.SystemPromptOverride,
			Background:      spec.Background,
			Source:          "built-in",
		}
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		l.loadDir(filepath.Join(home, ".swiftx", "agents"), "user")
	}

	if l.workDir != "" {
		l.loadDir(filepath.Join(l.workDir, ".swiftx", "agents"), "project")
	}

	return nil
}

func (l *AgentLoader) loadDir(dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		def, err := ParseAgentFile(path)
		if err != nil {
			msg := fmt.Sprintf("%s: %v", path, err)
			l.FailedFiles = append(l.FailedFiles, msg)
			if l.ErrorWriter != nil {
				fmt.Fprintf(l.ErrorWriter, "[swiftx] agent definition skipped — %s\n", msg)
			}
			continue
		}
		def.Source = source
		l.agents[def.AgentType] = def
	}
}

func (l *AgentLoader) Get(agentType string) *AgentDefinition {
	return l.agents[agentType]
}

func (l *AgentLoader) ListNames() []string {
	names := make([]string, 0, len(l.agents))
	for name := range l.agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
