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

package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const maxEntries = 200

type entry struct {
	Text string `json:"text"`
	Ts   int64  `json:"ts"`
}

func historyFilePath(dir string) string {
	return filepath.Join(dir, ".swiftx", "prompt_history.jsonl")
}

func Load(dir string) []string {
	path := historyFilePath(dir)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var texts []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e entry
		if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Text != "" {
			texts = append(texts, e.Text)
		}
	}
	return texts
}

func Append(dir string, text string) {
	path := historyFilePath(dir)
	os.MkdirAll(filepath.Dir(path), 0o755)

	existing := Load(dir)

	if len(existing) > 0 && existing[len(existing)-1] == text {
		return
	}

	existing = append(existing, text)
	if len(existing) > maxEntries {
		existing = existing[len(existing)-maxEntries:]
	}

	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, t := range existing {
		data, _ := json.Marshal(entry{Text: t, Ts: time.Now().Unix()})
		w.Write(data)
		w.WriteByte('\n')
	}
	w.Flush()
}
