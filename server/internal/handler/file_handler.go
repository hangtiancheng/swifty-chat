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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/config"

	"github.com/hangtiancheng/swifty.go/swifty_http"
)

// Chunked upload endpoints migrated from the legacy backend: verify enables
// instant upload (秒传) and resumable uploads, upload-chunk stores one slice,
// merge assembles the final file under /static/files.
const maxChunkSize = 10 << 20 // 10 MiB per chunk

var (
	fileHashPattern = regexp.MustCompile(`^[a-fA-F0-9]{8,64}$`)
	extNamePattern  = regexp.MustCompile(`^[a-zA-Z0-9]{1,10}$`)
)

func validateHashExt(fileHash, extName string) bool {
	return fileHashPattern.MatchString(fileHash) && extNamePattern.MatchString(extName)
}

func mergedFileName(fileHash, extName string) string {
	return strings.ToLower(fileHash) + "." + strings.ToLower(extName)
}

func chunkDir(fileHash string) string {
	return filepath.Join(config.Get().Static.ChunkPath, strings.ToLower(fileHash))
}

// VerifyFile reports whether the file already exists (instant upload) or
// which chunk indexes are still missing (resumable upload).
func VerifyFile(ctx *swifty_http.Context, next func()) {
	var req struct {
		FileHash string `json:"file_hash"`
		ChunkCnt int    `json:"chunk_cnt"`
		ExtName  string `json:"ext_name"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	if !validateHashExt(req.FileHash, req.ExtName) || req.ChunkCnt <= 0 {
		JsonBack(ctx, "invalid file_hash, ext_name or chunk_cnt", -2, nil)
		return
	}

	conf := config.Get()
	finalName := mergedFileName(req.FileHash, req.ExtName)
	if _, err := os.Stat(filepath.Join(conf.Static.FilePath, finalName)); err == nil {
		JsonBack(ctx, "file already uploaded", 0, swifty_http.H{
			"uploaded": true,
			"url":      "/static/files/" + finalName,
		})
		return
	}

	pending := make([]int, 0, req.ChunkCnt)
	dir := chunkDir(req.FileHash)
	for i := 0; i < req.ChunkCnt; i++ {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("chunk-%d", i))); err != nil {
			pending = append(pending, i)
		}
	}
	JsonBack(ctx, "success", 0, swifty_http.H{
		"uploaded":       false,
		"pending_chunks": pending,
	})
}

// UploadChunk stores a single multipart chunk under the chunk directory.
func UploadChunk(ctx *swifty_http.Context, next func()) {
	fileHash := ctx.PostForm("file_hash")
	extName := ctx.PostForm("ext_name")
	chunkIdxRaw := ctx.PostForm("chunk_idx")
	chunkIdx, err := strconv.Atoi(chunkIdxRaw)
	if err != nil || chunkIdx < 0 || !validateHashExt(fileHash, extName) {
		JsonBack(ctx, "invalid chunk metadata", -2, nil)
		return
	}

	file, header, err := ctx.FormFile("chunk")
	if err != nil {
		JsonBack(ctx, "chunk is required", -2, nil)
		return
	}
	defer file.Close()
	if header.Size > maxChunkSize {
		JsonBack(ctx, fmt.Sprintf("chunk too large (max %d MB)", maxChunkSize>>20), -2, nil)
		return
	}

	dir := chunkDir(fileHash)
	if err := os.MkdirAll(dir, 0755); err != nil {
		JsonBack(ctx, "failed to save chunk", -1, nil)
		return
	}
	dst := filepath.Join(dir, fmt.Sprintf("chunk-%d", chunkIdx))
	out, err := os.Create(dst)
	if err != nil {
		JsonBack(ctx, "failed to save chunk", -1, nil)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(file, maxChunkSize)); err != nil {
		JsonBack(ctx, "failed to save chunk", -1, nil)
		return
	}
	JsonBack(ctx, "chunk uploaded", 0, nil)
}

// MergeFile concatenates the uploaded chunks into the final file and removes
// the chunk directory.
func MergeFile(ctx *swifty_http.Context, next func()) {
	var req struct {
		FileHash string `json:"file_hash"`
		ExtName  string `json:"ext_name"`
		FileName string `json:"file_name"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	if !validateHashExt(req.FileHash, req.ExtName) {
		JsonBack(ctx, "invalid file_hash or ext_name", -2, nil)
		return
	}

	conf := config.Get()
	finalName := mergedFileName(req.FileHash, req.ExtName)
	finalPath := filepath.Join(conf.Static.FilePath, finalName)
	respond := func(size int64) {
		JsonBack(ctx, "merge successful", 0, swifty_http.H{
			"url":       "/static/files/" + finalName,
			"file_name": req.FileName,
			"file_size": fmt.Sprintf("%d", size),
		})
	}

	if info, err := os.Stat(finalPath); err == nil {
		respond(info.Size())
		return
	}

	dir := chunkDir(req.FileHash)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		JsonBack(ctx, "no chunks to merge", -2, nil)
		return
	}
	type chunk struct {
		idx  int
		name string
	}
	var chunks []chunk
	for _, e := range entries {
		idxRaw, ok := strings.CutPrefix(e.Name(), "chunk-")
		if !ok {
			continue
		}
		idx, err := strconv.Atoi(idxRaw)
		if err != nil {
			continue
		}
		chunks = append(chunks, chunk{idx: idx, name: e.Name()})
	}
	if len(chunks) == 0 {
		JsonBack(ctx, "no chunks to merge", -2, nil)
		return
	}
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].idx < chunks[j].idx })

	if err := os.MkdirAll(conf.Static.FilePath, 0755); err != nil {
		JsonBack(ctx, "failed to merge file", -1, nil)
		return
	}
	out, err := os.Create(finalPath)
	if err != nil {
		JsonBack(ctx, "failed to merge file", -1, nil)
		return
	}
	var total int64
	for _, c := range chunks {
		in, err := os.Open(filepath.Join(dir, c.name))
		if err != nil {
			out.Close()
			_ = os.Remove(finalPath)
			JsonBack(ctx, "failed to merge file", -1, nil)
			return
		}
		n, err := io.Copy(out, in)
		in.Close()
		if err != nil {
			out.Close()
			_ = os.Remove(finalPath)
			JsonBack(ctx, "failed to merge file", -1, nil)
			return
		}
		total += n
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(finalPath)
		JsonBack(ctx, "failed to merge file", -1, nil)
		return
	}
	_ = os.RemoveAll(dir)
	respond(total)
}
