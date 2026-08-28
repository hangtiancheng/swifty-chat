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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Teammates exchange several structured message types in addition to plain
// text. Each carries a RequestID that the response echoes back verbatim, so
// the Lead can match a response to the request it sent: when sending shutdown
// requests to three teammates simultaneously, the three responses are
// indistinguishable without the ID.
const (
	// MsgText is a plain text message, appended directly to the teammate's next prompt.
	MsgText = "text"
	// MsgShutdownRequest is initiated by the Lead, asking a teammate to wrap up.
	// The teammate may refuse.
	MsgShutdownRequest = "shutdown_request"
	// MsgShutdownResponse is the teammate's reply to a shutdown request;
	// Approve == false means work is not yet finished.
	MsgShutdownResponse = "shutdown_response"
	// MsgPlanApprovalRequest is initiated by a teammate, submitting a plan for
	// the Lead's approval.
	MsgPlanApprovalRequest = "plan_approval_request"
	// MsgPlanApprovalResponse is the Lead's approval decision; on rejection,
	// Text carries revision feedback.
	MsgPlanApprovalResponse = "plan_approval_response"
)

// NewRequestID generates a request identifier. A random string is used instead
// of an auto-incrementing sequence because requests may originate from
// teammates in different processes, where sequences would collide.
func NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-0"
	}
	return "req-" + hex.EncodeToString(b[:])
}

// NewShutdownRequest constructs a shutdown request. Text carries the reason,
// which the teammate uses to decide whether to agree.
func NewShutdownRequest(from, reason string) FileMailMessage {
	if reason == "" {
		reason = "team is wrapping up"
	}
	return newTyped(from, MsgShutdownRequest, NewRequestID(), fmt.Sprintf("[shutdown] %s", reason))
}

// NewShutdownResponse constructs the teammate's reply to a shutdown request.
func NewShutdownResponse(from, requestID string, approve bool, reason string) FileMailMessage {
	m := newTyped(from, MsgShutdownResponse, requestID, reason)
	m.Approve = &approve
	return m
}

// NewPlanApprovalRequest constructs a plan approval request; Text is the full plan.
func NewPlanApprovalRequest(from, plan string) FileMailMessage {
	return newTyped(from, MsgPlanApprovalRequest, NewRequestID(), plan)
}

// NewPlanApprovalResponse constructs an approval decision; on rejection,
// feedback describes what needs to change.
func NewPlanApprovalResponse(from, requestID string, approve bool, feedback string) FileMailMessage {
	m := newTyped(from, MsgPlanApprovalResponse, requestID, feedback)
	m.Approve = &approve
	return m
}

func newTyped(from, msgType, requestID, text string) FileMailMessage {
	m := NewFileMailMessage(from, text)
	m.Type = msgType
	m.RequestID = requestID
	return m
}

// IsShutdownRequest reports whether a message is a shutdown request.
//
// In addition to checking Type, it also recognizes the "[shutdown]" text
// prefix: pane teammates are independent processes that may have been started
// by an older version; and a user manually inserting a line into the mailbox
// should also work.
func IsShutdownRequest(m FileMailMessage) bool {
	return m.Type == MsgShutdownRequest || strings.HasPrefix(strings.TrimSpace(m.Text), ShutdownPrefix)
}

// Approved reports whether the response indicates agreement. When the field
// is absent, it defaults to not-approved — better to make the Lead wait an
// extra turn than to treat silence as consent.
func (m FileMailMessage) Approved() bool {
	return m.Approve != nil && *m.Approve
}
