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

func spawnTmuxTeammate(teamName, memberName, cliCommand string) (string, error) {
	paneName := fmt.Sprintf("%s-%s", teamName, memberName)

	// Create a new tmux window (not split) for the teammate
	cmd := exec.Command("tmux", "new-window", "-d", "-n", paneName, cliCommand)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("tmux new-window: %s: %s", err, strings.TrimSpace(string(output)))
	}
	return paneName, nil
}

func stopTmuxTeammate(paneName string) {
	exec.Command("tmux", "send-keys", "-t", paneName, "C-c", "").Run()
	exec.Command("tmux", "kill-window", "-t", paneName).Run()
}
