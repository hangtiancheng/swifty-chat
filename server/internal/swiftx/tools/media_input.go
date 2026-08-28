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
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// inputImageRawBudget is the largest JPEG byte size we'll pack into a data URI
// for img2img. Agnes accepts data URIs up to ~80KB total length; base64 inflates
// the payload by ~33%, so capping the raw JPEG at 50KB keeps the final URI
// comfortably under that threshold.
const inputImageRawBudget = 50 * 1024

// prepareInputImage normalizes one input_images entry for the upstream API.
//
//   - http(s):// URLs pass through unchanged — the provider fetches them itself
//     and there's no useful size cap to enforce on a URL.
//   - data: URIs and local file paths get decoded, downscaled, and re-encoded as
//     JPEG until they fit inputImageRawBudget, then returned as a data URI.
//
// workDir is used to resolve relative paths. Returns ("", err) on unreadable
// files or undecodable images.
func prepareInputImage(in, workDir string) (string, error) {
	if in == "" {
		return "", errors.New("empty input image")
	}
	low := strings.ToLower(in)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return in, nil
	}

	var raw []byte
	switch {
	case strings.HasPrefix(low, "data:"):
		b, err := decodeDataURI(in)
		if err != nil {
			return "", err
		}
		raw = b
	default:
		path := in
		if !filepath.IsAbs(path) && workDir != "" {
			path = filepath.Join(workDir, path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read input image %q: %w", in, err)
		}
		raw = b
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("decode input image: %w", err)
	}

	jpegBytes, err := shrinkToBudget(img, inputImageRawBudget)
	if err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegBytes), nil
}

// shrinkToBudget JPEG-encodes img, lowering quality first and then halving
// dimensions, until the encoded byte count is <= budget. Returns the smallest
// encoded form we managed to produce; only errors out if even an 8x8 thumbnail
// can't fit (which would mean budget is absurdly small).
func shrinkToBudget(img image.Image, budget int) ([]byte, error) {
	qualities := []int{82, 65, 45, 28}
	for {
		for _, q := range qualities {
			buf := &bytes.Buffer{}
			if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: q}); err != nil {
				return nil, fmt.Errorf("encode jpeg: %w", err)
			}
			if buf.Len() <= budget {
				return buf.Bytes(), nil
			}
		}
		b := img.Bounds()
		w, h := b.Dx()/2, b.Dy()/2
		if w < 8 || h < 8 {
			return nil, fmt.Errorf("cannot shrink image under %d bytes", budget)
		}
		img = halve(img)
	}
}

// halve returns img downscaled to half width and half height using 2x2 box
// averaging. Box filtering is cheap and produces clean downsamples — exact
// reconstruction isn't needed since the result feeds an img2img model.
func halve(src image.Image) image.Image {
	sb := src.Bounds()
	w, h := sb.Dx()/2, sb.Dy()/2
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			sx, sy := sb.Min.X+x*2, sb.Min.Y+y*2
			r1, g1, b1, a1 := src.At(sx, sy).RGBA()
			r2, g2, b2, a2 := src.At(sx+1, sy).RGBA()
			r3, g3, b3, a3 := src.At(sx, sy+1).RGBA()
			r4, g4, b4, a4 := src.At(sx+1, sy+1).RGBA()
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8((r1 + r2 + r3 + r4) >> 10),
				G: uint8((g1 + g2 + g3 + g4) >> 10),
				B: uint8((b1 + b2 + b3 + b4) >> 10),
				A: uint8((a1 + a2 + a3 + a4) >> 10),
			})
		}
	}
	return dst
}

// validateVideoInputURL enforces that a generate_video input image is a public
// http(s) URL. The Agnes video endpoint accepts URLs only — local paths and
// data URIs trip its base64 / fetch handling and the task fails async with a
// confusing "Invalid image" error, which used to read to callers as a generic
// "parse error" retry loop.
func validateVideoInputURL(in string) error {
	if in == "" {
		return errors.New("empty input image")
	}
	low := strings.ToLower(in)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return nil
	}
	if strings.HasPrefix(low, "data:") {
		return fmt.Errorf("video input_images must be public http(s) URLs; data URIs are not accepted by the upstream API")
	}
	return fmt.Errorf("video input_images must be public http(s) URLs (got %q); upload the image to a reachable URL first", in)
}

// decodeDataURI returns the raw bytes from a "data:[mime];base64,..." string.
// Non-base64 data URIs are rejected since the API only round-trips base64.
func decodeDataURI(s string) ([]byte, error) {
	before, after, ok := strings.Cut(s, ",")
	if !ok {
		return nil, errors.New("data URI missing comma separator")
	}
	header := before
	if !strings.Contains(header, ";base64") {
		return nil, errors.New("data URI must be base64-encoded")
	}
	return base64.StdEncoding.DecodeString(after)
}
