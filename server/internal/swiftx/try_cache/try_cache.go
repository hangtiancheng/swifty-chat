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
