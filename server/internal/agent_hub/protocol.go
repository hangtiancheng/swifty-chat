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

// Package agent_hub runs one private swiftx agent per chat user and streams its
// progress to the browser over a dedicated websocket.
//
// Prompts do not arrive here directly: they travel the normal chat pipeline, so
// they are persisted, echoed and reflected in session lists like any other
// message, and only then get dispatched to the owning agent. This socket
// carries the parts of a run that have no place in a chat transcript — token
// deltas, thinking, tool calls, permission prompts — plus the control replies
// those prompts need.
package agent_hub

import "encoding/json"

// Event is one downstream frame, server to browser.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// clientFrame is one upstream frame, browser to server.
type clientFrame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type permissionReply struct {
	ID       string `json:"id"`
	Response string `json:"response"` // allow / deny / allowAlways
}

type askUserReply struct {
	ID      string            `json:"id"`
	Answers map[string]string `json:"answers"`
}

// CommandInfo describes one slash command for the composer's command menu.
type CommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ChatSink writes finalized agent text back into the chat transcript, which is
// what makes replies survive a reload and show up in session previews.
type ChatSink interface {
	// SaveAssistantText stores one finalized text block as a chat message from
	// the assistant to userID, tagged with the conversation it belongs to, and
	// returns the new message uuid. An empty return means persistence failed
	// and the block exists only on screen.
	SaveAssistantText(userID, sessionID, text string) string
}
