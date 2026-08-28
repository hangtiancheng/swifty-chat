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

//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"strings"
)

// sandboxExecPath is a hardcoded path to prevent PATH injection attacks.
const sandboxExecPath = "/usr/bin/sandbox-exec"

// darwinSandbox implements sandbox isolation using macOS seatbelt (sandbox-exec).
// It dynamically generates a seatbelt profile to control file-write and network
// access permissions.
type darwinSandbox struct{}

func newPlatformSandbox() Sandbox {
	return &darwinSandbox{}
}

func (s *darwinSandbox) Available() bool {
	_, err := os.Stat(sandboxExecPath)
	return err == nil
}

// buildProfile dynamically generates a seatbelt profile string.
// Strategy: deny by default -> allow exec/read -> allow write per path -> deny write per path -> network control.
func buildProfile(config Config) string {
	var sb strings.Builder

	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n")

	// Allow process execution and fork.
	sb.WriteString("(allow process-exec)\n")
	sb.WriteString("(allow process-fork)\n")
	// Allow reading system information.
	sb.WriteString("(allow sysctl-read)\n")
	// Full-disk readable.
	sb.WriteString("(allow file-read* (subpath \"/\"))\n")

	// Allow write per path.
	for _, path := range config.AllowWrite {
		fmt.Fprintf(&sb, "(allow file-write* (subpath %q))\n", path)
	}

	// Paths to deny write are placed after allow rules; seatbelt applies
	// later rules with higher priority. Single files use literal for exact
	// match; directories use subpath for prefix match.
	for _, path := range config.DenyWrite {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			fmt.Fprintf(&sb, "(deny file-write* (subpath %q))\n", path)
		} else {
			fmt.Fprintf(&sb, "(deny file-write* (literal %q))\n", path)
		}
	}

	// Network control.
	if config.NetworkEnabled {
		sb.WriteString("(allow network*)\n")
	} else {
		sb.WriteString("(deny network*)\n")
	}

	return sb.String()
}

func (s *darwinSandbox) Wrap(command string, config Config) (string, error) {
	profile := buildProfile(config)
	// Pass profile via -p flag; wrap command in single quotes to prevent
	// shell from re-interpreting it.
	wrapped := fmt.Sprintf("%s -p '%s' bash -c %q", sandboxExecPath, profile, command)
	return wrapped, nil
}
