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

package teams

import (
	"fmt"
	"os/exec"
	"strings"
)

// ModeITerm spawns each teammate in its own iTerm2 tab via AppleScript.
// macOS-only; detectBackend selects it when ITERM_SESSION_ID is set.
const ModeITerm TeamMode = "iterm"

// spawnITermTeammate opens a new iTerm2 tab and runs cliCommand in it.
// Returns the script-side tab identifier ("team-member") so the caller can
// later target it for shutdown.
func spawnITermTeammate(teamName, memberName, cliCommand string) (string, error) {
	tabName := fmt.Sprintf("%s-%s", teamName, memberName)
	// Escape any embedded double quotes so the AppleScript string literal stays valid.
	safeCmd := strings.ReplaceAll(cliCommand, `"`, `\"`)
	safeName := strings.ReplaceAll(tabName, `"`, `\"`)
	script := fmt.Sprintf(`tell application "iTerm2"
  tell current window
    set newTab to create tab with default profile
    tell newTab
      set name to "%s"
      tell current session
        write text "%s"
      end tell
    end tell
  end tell
end tell`, safeName, safeCmd)

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("osascript: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return tabName, nil
}

// stopITermTeammate closes the iTerm2 tab created by spawnITermTeammate.
// Best-effort: missing tab / closed window are not reported as errors.
func stopITermTeammate(tabName string) {
	safeName := strings.ReplaceAll(tabName, `"`, `\"`)
	script := fmt.Sprintf(`tell application "iTerm2"
  repeat with w in windows
    repeat with t in tabs of w
      if name of t is "%s" then
        tell t to close
      end if
    end repeat
  end repeat
end tell`, safeName)
	exec.Command("osascript", "-e", script).Run()
}
