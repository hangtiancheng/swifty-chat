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

// Package try_cache records process startup, shutdown, and crash diagnostics for post-mortem analysis after abnormal exits.
package try_cache

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

const logDir = ".swiftx"

func logPath() string {
	return filepath.Join(logDir, "crash.log")
}

// Record appends a timestamped entry to the crash log.
// Diagnostics must never crash the process themselves, so write failures are silently discarded.
func Record(text string) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), text)
}

// RecordPanic logs a panic with its full stack trace. The context parameter identifies which layer the crash originated from.
func RecordPanic(context string, value any, stack []byte) {
	Record(fmt.Sprintf("crash [%s] %v\n%s", context, value, stack))
}

// TryCatch installs crash diagnostics. Call it once at process startup; the returned
// function should be called before exit to leave an exit marker.
//
// Three kinds of traces are recorded: a "start" line marks the beginning of a run;
// an "exit" line is written when main returns normally; SetCrashOutput redirects
// runtime-level crash output to the log file — panics in goroutines, concurrent map
// writes, and deadlock detection produce fatal errors that bypass recover, so only a
// file redirect preserves the crash scene for later inspection.
// Combining all three determines the exit mode: crash + exit means the process
// crashed; start + exit alone means a clean shutdown; start alone means the process
// was killed externally before it could leave any trace.
func TryCatch() func() {
	Record(fmt.Sprintf("start pid=%d", os.Getpid()))
	if err := os.MkdirAll(logDir, 0o755); err == nil {
		f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			// SetCrashOutput duplicates the file descriptor internally,
			// so we can close our handle immediately.
			_ = debug.SetCrashOutput(f, debug.CrashOptions{})
			_ = f.Close()
		}
	}
	return func() {
		Record(fmt.Sprintf("exit pid=%d", os.Getpid()))
	}
}
