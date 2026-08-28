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

//go:build linux

package sandbox

import (
	"fmt"
	"os/exec"
	"strings"
)

// linuxSandbox implements sandbox isolation using bubblewrap (bwrap).
// bwrap leverages Linux user namespaces to create lightweight isolated environments.
type linuxSandbox struct{}

func newPlatformSandbox() Sandbox {
	return &linuxSandbox{}
}

func (s *linuxSandbox) Available() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

func (s *linuxSandbox) Wrap(command string, config Config) (string, error) {
	var args []string

	// Isolate user and pid namespaces.
	args = append(args, "bwrap", "--unshare-user", "--unshare-pid")

	// Read-only bind mount of the root filesystem.
	args = append(args, "--ro-bind", "/", "/")

	// Allow write per path (writable bind mounts).
	for _, path := range config.AllowWrite {
		args = append(args, "--bind", path, path)
	}

	// Force read-only (overrides writable sub-paths under root).
	for _, path := range config.DenyWrite {
		args = append(args, "--ro-bind", path, path)
	}

	// Network isolation.
	if !config.NetworkEnabled {
		args = append(args, "--unshare-net")
	}

	// Mount /proc; many commands depend on it.
	args = append(args, "--proc", "/proc")

	// Append the command to execute.
	args = append(args, "--", "bash", "-c", command)

	// Assemble the full command string; shell-special characters must be
	// properly escaped.
	var sb strings.Builder
	for i, arg := range args {
		if i > 0 {
			sb.WriteByte(' ')
		}
		// Wrap arguments containing spaces or special characters in single quotes.
		if strings.ContainsAny(arg, " \t\n\"'\\$`!") {
			sb.WriteString(fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "'\\''")))
		} else {
			sb.WriteString(arg)
		}
	}
	return sb.String(), nil
}
