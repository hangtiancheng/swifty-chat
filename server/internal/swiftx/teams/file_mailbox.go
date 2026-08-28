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
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// lockAcquireTimeout is the total time limit for waiting on the file lock.
	// On timeout, return an error for the caller to handle — never silently
	// discard the message.
	lockAcquireTimeout = 5 * time.Second
	// staleLockAge: a lock file older than this duration is considered
	// abandoned by a crashed holder and may be forcibly taken over.
	staleLockAge = 10 * time.Second
	// maxLockBackoff caps the backoff to prevent unbounded growth under high
	// concurrency.
	maxLockBackoff = 80 * time.Millisecond
)

type FileMailBox struct {
	baseDir string
	// Intra-process concurrency is serialized with an in-memory lock; the file
	// lock only isolates teammates in separate processes. This avoids a round
	// of filesystem contention and prevents same-process goroutines from
	// exhausting each other's retry budget.
	mu sync.Mutex
}

type FileMailMessage struct {
	From      string `json:"from"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
	Color     string `json:"color,omitempty"`

	// Three fields for structured messages; left empty for plain text messages.
	// Type: see the Msg* constants in protocol.go; RequestID: allows responses
	// to be matched to requests; Approve uses a pointer to distinguish "no
	// response" from "explicitly rejected".
	Type      string `json:"type,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Approve   *bool  `json:"approve,omitempty"`
}

// NewFileMailMessage constructs a plain text message with an RFC3339Nano timestamp.
func NewFileMailMessage(from, text string) FileMailMessage {
	return FileMailMessage{
		From:      from,
		Text:      text,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func NewFileMailBox(baseDir string) *FileMailBox {
	os.MkdirAll(baseDir, 0755)
	return &FileMailBox{baseDir: baseDir}
}

func (mb *FileMailBox) inboxPath(agentID string) string {
	return filepath.Join(mb.baseDir, agentID+".json")
}

func (mb *FileMailBox) lockPath(agentID string) string {
	return filepath.Join(mb.baseDir, agentID+".json.lock")
}

func (mb *FileMailBox) Send(recipient string, msg FileMailMessage) error {
	return mb.withLock(recipient, func(messages []FileMailMessage) ([]FileMailMessage, error) {
		msg.Read = false
		if msg.Timestamp == "" {
			msg.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
		}
		return append(messages, msg), nil
	})
}

func (mb *FileMailBox) ReadUnread(agentID string) ([]FileMailMessage, error) {
	messages, err := mb.readInbox(agentID)
	if err != nil {
		return nil, err
	}
	var unread []FileMailMessage
	for _, m := range messages {
		if !m.Read {
			unread = append(unread, m)
		}
	}
	return unread, nil
}

func (mb *FileMailBox) MarkAllRead(agentID string) error {
	return mb.withLock(agentID, func(messages []FileMailMessage) ([]FileMailMessage, error) {
		for i := range messages {
			messages[i].Read = true
		}
		return messages, nil
	})
}

// withLock acquires a file lock, reads the inbox, applies the mutation, and writes back.
func (mb *FileMailBox) withLock(agentID string, fn func([]FileMailMessage) ([]FileMailMessage, error)) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	lockFile := mb.lockPath(agentID)

	// Acquire the file lock: backoff grows exponentially with jitter to avoid
	// multiple processes waking at the same instant and colliding repeatedly.
	// If the lock cannot be acquired within the total time limit, return an
	// error so the caller knows the message was not written.
	var lockFd *os.File
	var err error
	deadline := time.Now().Add(lockAcquireTimeout)
	backoff := 5 * time.Millisecond
	for {
		lockFd, err = os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return err
		}
		// Lock is held by another process; check if it is stale enough to take over.
		if info, statErr := os.Stat(lockFile); statErr == nil {
			if time.Since(info.ModTime()) > staleLockAge {
				os.Remove(lockFile)
				continue
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("mailbox %s: waited for file lock longer than %s, message not written", agentID, lockAcquireTimeout)
		}
		time.Sleep(backoff + time.Duration(rand.Int63n(int64(backoff))))
		if backoff < maxLockBackoff {
			backoff *= 2
		}
	}
	lockFd.Close()
	defer os.Remove(lockFile)

	// Re-read inbox after acquiring lock
	messages, _ := mb.readInbox(agentID)

	// Apply mutation
	messages, err = fn(messages)
	if err != nil {
		return err
	}

	// Write back
	return mb.writeInbox(agentID, messages)
}

func (mb *FileMailBox) readInbox(agentID string) ([]FileMailMessage, error) {
	path := mb.inboxPath(agentID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var messages []FileMailMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, nil
	}
	return messages, nil
}

func (mb *FileMailBox) writeInbox(agentID string, messages []FileMailMessage) error {
	path := mb.inboxPath(agentID)
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
