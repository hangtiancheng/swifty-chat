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

package handler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/util"

	"github.com/hangtiancheng/swifty.go/swifty_http"
)

const (
	maxAvatarSize = 5 << 20  // 5 MiB
	maxFileSize   = 50 << 20 // 50 MiB
)

var avatarExtWhitelist = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

// sanitizeFilename strips any path components and keeps only a safe
// character set, preventing path traversal via the uploaded filename.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	s := strings.Trim(b.String(), ".")
	if s == "" {
		s = "file"
	}
	return s
}

func saveUpload(ctx *swifty_http.Context, dir string, maxSize int64, extWhitelist map[string]bool) (url string, origName string, size int64, ok bool) {
	file, header, err := ctx.FormFile("file")
	if err != nil {
		JsonBack(ctx, "file is required", -2, nil)
		return "", "", 0, false
	}
	defer file.Close()

	if header.Size > maxSize {
		JsonBack(ctx, fmt.Sprintf("file too large (max %d MB)", maxSize>>20), -2, nil)
		return "", "", 0, false
	}

	safeName := sanitizeFilename(header.Filename)
	if extWhitelist != nil {
		ext := strings.ToLower(filepath.Ext(safeName))
		if !extWhitelist[ext] {
			JsonBack(ctx, "unsupported file type", -2, nil)
			return "", "", 0, false
		}
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		JsonBack(ctx, "failed to save file", -1, nil)
		return "", "", 0, false
	}

	filename := fmt.Sprintf("%s_%s", util.GetNowAndLenRandomString(8), safeName)
	dst := filepath.Join(dir, filename)

	out, err := os.Create(dst)
	if err != nil {
		JsonBack(ctx, "failed to save file", -1, nil)
		return "", "", 0, false
	}
	defer out.Close()

	if _, err := io.Copy(out, io.LimitReader(file, maxSize)); err != nil {
		JsonBack(ctx, "failed to save file", -1, nil)
		return "", "", 0, false
	}
	return filename, header.Filename, header.Size, true
}

func UploadAvatar(ctx *swifty_http.Context, next func()) {
	conf := config.Get()
	filename, _, _, ok := saveUpload(ctx, conf.Static.AvatarPath, maxAvatarSize, avatarExtWhitelist)
	if !ok {
		return
	}
	JsonBack(ctx, "upload successful", 0, swifty_http.H{"url": "/static/avatars/" + filename})
}

func UploadFile(ctx *swifty_http.Context, next func()) {
	conf := config.Get()
	filename, origName, size, ok := saveUpload(ctx, conf.Static.FilePath, maxFileSize, nil)
	if !ok {
		return
	}
	JsonBack(ctx, "upload successful", 0, swifty_http.H{
		"url":       "/static/files/" + filename,
		"file_name": origName,
		"file_size": fmt.Sprintf("%d", size),
	})
}
