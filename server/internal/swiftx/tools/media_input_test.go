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

package tools

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareInputImage_URLPassthrough(t *testing.T) {
	for _, u := range []string{
		"https://example.com/a.png",
		"http://example.com/b.jpg",
		"HTTPS://EXAMPLE.COM/c.png",
	} {
		out, err := prepareInputImage(u, "")
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		if out != u {
			t.Errorf("URL was rewritten: %q -> %q", u, out)
		}
	}
}

func TestPrepareInputImage_LocalFileShrinksUnderBudget(t *testing.T) {
	dir := t.TempDir()
	// Random-pixel PNG defeats compression, forcing the shrinker to actually
	// downscale rather than just lowering JPEG quality.
	path := filepath.Join(dir, "big.png")
	writeRandomPNG(t, path, 1024, 1536)

	out, err := prepareInputImage(path, "")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !strings.HasPrefix(out, "data:image/jpeg;base64,") {
		t.Fatalf("expected jpeg data URI, got prefix %q", out[:40])
	}
	// Upstream rejects URIs much over ~80KB; assert we stayed well under that.
	if len(out) > 80*1024 {
		t.Errorf("data URI is %d bytes, want <= 80KB", len(out))
	}
	// And the bytes must still decode to a valid JPEG.
	b64 := strings.TrimPrefix(out, "data:image/jpeg;base64,")
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if _, _, err := image.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("re-decode output jpeg: %v", err)
	}
}

func TestPrepareInputImage_RelativePathResolvesAgainstWorkDir(t *testing.T) {
	dir := t.TempDir()
	writeTinyPNG(t, filepath.Join(dir, "ref.png"))
	out, err := prepareInputImage("ref.png", dir)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !strings.HasPrefix(out, "data:image/jpeg;base64,") {
		t.Fatalf("unexpected output: %q", out[:40])
	}
}

func TestPrepareInputImage_DataURIIsReprocessed(t *testing.T) {
	// Encode a random 800x800 PNG as a data URI; without re-encoding it would
	// blow past the 80KB cap.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	writeRandomPNG(t, path, 800, 800)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	in := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)

	out, err := prepareInputImage(in, "")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if len(out) > 80*1024 {
		t.Errorf("output URI is %d bytes, want <= 80KB", len(out))
	}
	if !strings.HasPrefix(out, "data:image/jpeg;base64,") {
		t.Errorf("expected jpeg output, got %q", out[:40])
	}
}

func TestPrepareInputImage_BadInputs(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"missing file":      "/nonexistent/path/does/not/exist.png",
		"non-image file":    "create-text-file",
		"non-base64 data":   "data:image/png,raw-bytes",
		"data URI no comma": "data:image/png;base64",
	}
	dir := t.TempDir()
	textFile := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(textFile, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases["non-image file"] = textFile

	for name, in := range cases {
		if _, err := prepareInputImage(in, ""); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func writeTinyPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range 16 {
		img.Pix[i*4+0] = uint8(i * 16)
		img.Pix[i*4+1] = 128
		img.Pix[i*4+2] = 200
		img.Pix[i*4+3] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func writeRandomPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(rng.Intn(256)),
				G: uint8(rng.Intn(256)),
				B: uint8(rng.Intn(256)),
				A: 255,
			})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
