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

package commands

// SandboxMode defines three sandbox operating modes
type SandboxMode int

const (
	SandboxAutoAllow SandboxMode = iota // Sandbox + auto-allow (recommended)
	SandboxRegular                      // Sandbox + regular permission confirmation
	SandboxOff                          // Disable sandbox
)

// SandboxModeLabels returns display labels for the three modes
func SandboxModeLabels() []string {
	return []string{
		"Enable Sandbox + Auto-Allow (Recommended)",
		"Enable Sandbox + Regular Permissions",
		"Disable Sandbox",
	}
}

// SandboxModeDescriptions returns explanatory text for each mode
func SandboxModeDescriptions() []string {
	return []string{
		"Commands execute inside a sandbox automatically, no confirmation needed. Explicit deny rules still apply.",
		"Commands execute inside a sandbox, but still require permission confirmation.",
		"No OS-level isolation; relies solely on application-layer permissions.",
	}
}
