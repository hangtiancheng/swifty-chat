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
	"strings"
	"testing"
)

func TestShutdownRequestRecognized(t *testing.T) {
	// Structured shutdown request
	req := NewShutdownRequest(LeadName, "wrapping up")
	if req.Type != MsgShutdownRequest {
		t.Errorf("Type = %q, want %q", req.Type, MsgShutdownRequest)
	}
	if req.RequestID == "" {
		t.Error("shutdown request must carry a RequestID, otherwise responses cannot be matched")
	}
	if !IsShutdownRequest(req) {
		t.Error("structured shutdown request should be recognized")
	}

	// Plain text prefix must also be recognized; pane teammates may be older-version processes.
	legacy := NewFileMailMessage(LeadName, "[shutdown] stop")
	if !IsShutdownRequest(legacy) {
		t.Error("[shutdown] text prefix should be recognized")
	}

	// Normal messages must not be misidentified.
	normal := NewFileMailMessage(LeadName, "keep working on the auth module")
	if IsShutdownRequest(normal) {
		t.Error("normal message was misidentified as a shutdown request")
	}
}

func TestShutdownResponseCarriesDecision(t *testing.T) {
	req := NewShutdownRequest(LeadName, "wrapping up")

	yes := NewShutdownResponse("alice", req.RequestID, true, "done")
	if !yes.Approved() {
		t.Error("an approving response should have Approved() == true")
	}
	if yes.RequestID != req.RequestID {
		t.Errorf("response RequestID = %q, should echo back %q", yes.RequestID, req.RequestID)
	}

	no := NewShutdownResponse("alice", req.RequestID, false, "still running tests")
	if no.Approved() {
		t.Error("a rejecting response should have Approved() == false")
	}

	// When no decision is expressed, treat as not-approved; never treat silence as consent.
	silent := NewFileMailMessage("alice", "")
	if silent.Approved() {
		t.Error("should not be treated as approved when Approve field is absent")
	}
}

func TestPlanApprovalRoundTrip(t *testing.T) {
	req := NewPlanApprovalRequest("alice", "1. Read the auth package\n2. Extract the interface")
	if req.Type != MsgPlanApprovalRequest || req.RequestID == "" {
		t.Fatalf("plan request constructed incorrectly: %+v", req)
	}
	if !strings.Contains(req.Text, "Extract the interface") {
		t.Error("full plan should be placed in Text")
	}

	rej := NewPlanApprovalResponse(LeadName, req.RequestID, false, "do not touch the handler layer")
	if rej.Approved() {
		t.Error("a rejection should not be approved")
	}
	if rej.Text != "do not touch the handler layer" {
		t.Errorf("rejection feedback should be in Text, got %q", rej.Text)
	}
	if rej.RequestID != req.RequestID {
		t.Error("response must echo back the original request's RequestID")
	}
}

func TestTypedFieldsSurviveJSON(t *testing.T) {
	// The mailbox is persisted to disk; fields must survive a serialization round-trip.
	req := NewShutdownRequest(LeadName, "wrapping up")
	resp := NewShutdownResponse("alice", req.RequestID, false, "not finished yet")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("serialization failed: %v", err)
	}
	var got FileMailMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("deserialization failed: %v", err)
	}
	if got.Type != MsgShutdownResponse || got.RequestID != req.RequestID {
		t.Errorf("type or request ID did not survive serialization: %+v", got)
	}
	if got.Approve == nil || *got.Approve {
		t.Errorf("Approve=false did not survive serialization: %+v", got.Approve)
	}
}

func TestRequestIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		id := NewRequestID()
		if seen[id] {
			t.Fatalf("request ID collision: %s", id)
		}
		seen[id] = true
	}
}
