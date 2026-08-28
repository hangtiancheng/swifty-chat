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

package service

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"
)

// callManager tracks active audio/video call rooms and per-user busy state.
// A 1v1 room id is derived from the participant pair; a group call uses the
// group uuid as its room id.
type callManager struct {
	mu    sync.Mutex
	rooms map[string]map[string]bool // roomId -> member uuids
	users map[string]string          // uuid -> roomId
}

var Calls = &callManager{
	rooms: make(map[string]map[string]bool),
	users: make(map[string]string),
}

// CallRoomId returns the room id for a conversation: the group uuid for group
// calls, otherwise a deterministic key built from the ordered user pair.
func CallRoomId(sendId, receiveId string) string {
	if len(receiveId) > 0 && receiveId[0] == 'G' {
		return receiveId
	}
	if len(sendId) > 0 && sendId[0] == 'G' {
		return sendId
	}
	a, b := sendId, receiveId
	if a > b {
		a, b = b, a
	}
	return "P:" + a + ":" + b
}

func (m *callManager) IsBusy(uuid string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.users[uuid] != ""
}

// InRoom reports whether the user currently belongs to the given room.
func (m *callManager) InRoom(roomId, uuid string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rooms[roomId][uuid]
}

// Join adds the user to a room. It fails when the user is already busy in a
// different room.
func (m *callManager) Join(roomId, uuid string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.users[uuid]; ok && cur != roomId {
		return false
	}
	if m.rooms[roomId] == nil {
		m.rooms[roomId] = make(map[string]bool)
	}
	m.rooms[roomId][uuid] = true
	m.users[uuid] = roomId
	return true
}

// Leave removes the user from their room and returns the room id plus the
// remaining members. Empty rooms are dissolved.
func (m *callManager) Leave(uuid string) (string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	roomId, ok := m.users[uuid]
	if !ok {
		return "", nil
	}
	delete(m.users, uuid)
	members := m.rooms[roomId]
	delete(members, uuid)
	var remaining []string
	for member := range members {
		remaining = append(remaining, member)
	}
	if len(members) == 0 {
		delete(m.rooms, roomId)
	}
	sort.Strings(remaining)
	return roomId, remaining
}

// Members lists the users currently in the room.
func (m *callManager) Members(roomId string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var members []string
	for member := range m.rooms[roomId] {
		members = append(members, member)
	}
	sort.Strings(members)
	return members
}

// GetCallers returns the members of a call room, excluding the caller itself.
func GetCallers(roomId, selfId string) []string {
	members := Calls.Members(roomId)
	var others []string
	for _, m := range members {
		if m != selfId {
			others = append(others, m)
		}
	}
	return others
}

// CanSeeCallRoom reports whether uuid may read a room's participant list: the
// user is already in the room, the room is their own 1v1 pair room, or the
// room is a group they belong to.
func CanSeeCallRoom(ctx context.Context, roomId, uuid string) bool {
	if uuid == "" || roomId == "" {
		return false
	}
	if Calls.InRoom(roomId, uuid) {
		return true
	}
	if strings.HasPrefix(roomId, "P:") {
		for _, part := range strings.Split(strings.TrimPrefix(roomId, "P:"), ":") {
			if part == uuid {
				return true
			}
		}
		return false
	}
	if roomId[0] != 'G' {
		return false
	}
	var group model.GroupInfo
	if err := dao.ActiveQuery(&group).Where("uuid", roomId).First(ctx, &group); err != nil {
		return false
	}
	for _, member := range group.Members {
		if member == uuid {
			return true
		}
	}
	return false
}
