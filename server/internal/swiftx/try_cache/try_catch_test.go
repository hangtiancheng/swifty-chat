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

package try_cache

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func readLog(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".swiftx", "crash.log"))
	if err != nil {
		t.Fatalf("read crash log: %v", err)
	}
	return string(data)
}

func TestRecordAppends(t *testing.T) {
	t.Chdir(t.TempDir())

	Record("start pid=1")
	RecordPanic("main", "boom", []byte("goroutine 1 [running]:"))

	log := readLog(t)
	if !strings.Contains(log, "start pid=1") {
		t.Errorf("start record missing: %s", log)
	}
	if !strings.Contains(log, "crash [main] boom") {
		t.Errorf("panic record missing: %s", log)
	}
	if !strings.Contains(log, "goroutine 1 [running]:") {
		t.Errorf("stack missing: %s", log)
	}
	// Append semantics: a later record must not overwrite an earlier one.
	if strings.Index(log, "start pid=1") > strings.Index(log, "crash [main]") {
		t.Error("records are not in append order")
	}
}

func TestInstallWritesStartAndExit(t *testing.T) {
	t.Chdir(t.TempDir())
	// The runtime holds the crash output file until process exit; unbind it before
	// the test ends, otherwise the temp directory cannot be cleaned up.
	t.Cleanup(func() { _ = debug.SetCrashOutput(nil, debug.CrashOptions{}) })

	recordExit := TryCatch()
	if got := readLog(t); !strings.Contains(got, "start pid=") {
		t.Errorf("start record missing: %s", got)
	}
	recordExit()

	log := readLog(t)
	if !strings.Contains(log, "exit pid=") {
		t.Errorf("exit record missing: %s", log)
	}
}
