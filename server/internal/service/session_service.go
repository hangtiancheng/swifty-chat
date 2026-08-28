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
	"encoding/json"
	"log"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/hangtiancheng/swifty-chat/server/internal/constant"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"
	"github.com/hangtiancheng/swifty-chat/server/internal/util"

	"github.com/hangtiancheng/swifty.go/swifty_orm"
)

type UserSessionItem struct {
	SessionId       string `json:"session_id"`
	Avatar          string `json:"avatar"`
	UserId          string `json:"user_id"`
	Username        string `json:"username"`
	LastMessage     string `json:"last_message"`
	LastMessageType int8   `json:"last_message_type"`
	LastMessageAt   string `json:"last_message_at"`
	LastMessageAtMs int64  `json:"last_message_at_ms"`
	UnreadCnt       int64  `json:"unread_cnt"`
}

type GroupSessionItem struct {
	SessionId       string `json:"session_id"`
	Avatar          string `json:"avatar"`
	GroupId         string `json:"group_id"`
	GroupName       string `json:"group_name"`
	LastMessage     string `json:"last_message"`
	LastMessageType int8   `json:"last_message_type"`
	LastMessageAt   string `json:"last_message_at"`
	LastMessageAtMs int64  `json:"last_message_at_ms"`
	UnreadCnt       int64  `json:"unread_cnt"`
}

type sessionMeta struct {
	lastMessage     string
	lastMessageType int8
	lastMessageAt   string
	lastMessageAtMs int64
	unreadCnt       int64
	sortKey         int64
}

// enrichSession computes the latest visible message and the unread count for
// one session. AV signaling frames (type 3) never surface in previews.
func enrichSession(ctx context.Context, ownerId string, s *model.Session) sessionMeta {
	meta := sessionMeta{sortKey: s.CreatedAt.UnixMilli()}

	latest := dao.Engine.Model(&model.Message{}).
		Where("type", "!=", constant.MessageAudioOrVideo).
		OrderBy("created_at", "desc")
	unread := dao.Engine.Model(&model.Message{}).
		Where("type", "!=", constant.MessageAudioOrVideo)

	if len(s.ReceiveId) > 0 && s.ReceiveId[0] == 'G' {
		latest.Where("receive_id", s.ReceiveId)
		unread.Where("receive_id", s.ReceiveId).Where("send_id", "!=", ownerId)
	} else {
		latest.Where(bson.M{"$or": bson.A{
			bson.M{"send_id": ownerId, "receive_id": s.ReceiveId},
			bson.M{"send_id": s.ReceiveId, "receive_id": ownerId},
		}})
		unread.Where("send_id", s.ReceiveId).Where("receive_id", ownerId)
	}

	var msg model.Message
	if err := latest.First(ctx, &msg); err == nil {
		meta.lastMessage = messagePreview(&msg)
		meta.lastMessageType = msg.Type
		meta.lastMessageAt = msg.CreatedAt.Format("2006-01-02 15:04:05")
		meta.lastMessageAtMs = msg.CreatedAt.UnixMilli()
		meta.sortKey = meta.lastMessageAtMs
	}

	if s.LastReadAt != nil {
		unread.Where("created_at", ">", *s.LastReadAt)
	}
	if cnt, err := unread.Count(ctx); err == nil {
		meta.unreadCnt = cnt
	}
	return meta
}

func messagePreview(m *model.Message) string {
	switch m.Type {
	case constant.MessageText:
		return m.Content
	case constant.MessageImage:
		return "[Image]"
	case constant.MessageVideo:
		return "[Video]"
	case constant.MessageFile:
		if m.FileName != "" {
			return "[File] " + m.FileName
		}
		return "[File]"
	default:
		return m.Content
	}
}

func GetUserSessionList(ctx context.Context, ownerId string) (string, []UserSessionItem, int) {
	sessions, err := loadSessions(ctx, ownerId)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	type entry struct {
		item UserSessionItem
		key  int64
	}
	var entries []entry
	for i := range sessions {
		s := &sessions[i]
		if len(s.ReceiveId) > 0 && s.ReceiveId[0] == 'U' {
			meta := enrichSession(ctx, ownerId, s)
			entries = append(entries, entry{key: meta.sortKey, item: UserSessionItem{
				SessionId: s.Uuid, Avatar: s.Avatar, UserId: s.ReceiveId, Username: s.ReceiveName,
				LastMessage: meta.lastMessage, LastMessageType: meta.lastMessageType,
				LastMessageAt: meta.lastMessageAt, LastMessageAtMs: meta.lastMessageAtMs,
				UnreadCnt: meta.unreadCnt,
			}})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].key > entries[j].key })
	var list []UserSessionItem
	for _, e := range entries {
		list = append(list, e.item)
	}
	return "success", list, 0
}

func GetGroupSessionList(ctx context.Context, ownerId string) (string, []GroupSessionItem, int) {
	sessions, err := loadSessions(ctx, ownerId)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	type entry struct {
		item GroupSessionItem
		key  int64
	}
	var entries []entry
	for i := range sessions {
		s := &sessions[i]
		if len(s.ReceiveId) > 0 && s.ReceiveId[0] == 'G' {
			meta := enrichSession(ctx, ownerId, s)
			entries = append(entries, entry{key: meta.sortKey, item: GroupSessionItem{
				SessionId: s.Uuid, Avatar: s.Avatar, GroupId: s.ReceiveId, GroupName: s.ReceiveName,
				LastMessage: meta.lastMessage, LastMessageType: meta.lastMessageType,
				LastMessageAt: meta.lastMessageAt, LastMessageAtMs: meta.lastMessageAtMs,
				UnreadCnt: meta.unreadCnt,
			}})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].key > entries[j].key })
	var list []GroupSessionItem
	for _, e := range entries {
		list = append(list, e.item)
	}
	return "success", list, 0
}

// invalidateSessionCacheByReceiver drops the cached session list of every
// user that has a session pointing at the given receiver.
func invalidateSessionCacheByReceiver(ctx context.Context, receiveId string) {
	var owners []string
	if err := dao.ActiveQuery(&model.Session{}).
		Where("receive_id", receiveId).
		Pluck(ctx, "send_id", &owners); err != nil {
		log.Printf("invalidateSessionCacheByReceiver %s failed: %v", receiveId, err)
		return
	}
	for _, owner := range owners {
		_ = dao.SessionListCache.Delete(ctx, owner)
	}
}

func OpenSession(ctx context.Context, sendId, receiveId string) (string, string, int) {
	if sendId == "" || receiveId == "" {
		return "send_id and receive_id are required", "", -2
	}
	var session model.Session
	err := dao.ActiveQuery(&session).
		Where("send_id", sendId).
		Where("receive_id", receiveId).
		First(ctx, &session)
	if err == nil {
		return "session created", session.Uuid, 0
	}
	return createSession(ctx, sendId, receiveId)
}

func createSession(ctx context.Context, sendId, receiveId string) (string, string, int) {
	session := model.Session{
		Uuid:      "S" + util.GetNowAndLenRandomString(11),
		SendId:    sendId,
		ReceiveId: receiveId,
		CreatedAt: time.Now(),
	}

	if receiveId[0] == 'U' {
		var user model.UserInfo
		if err := dao.ActiveQuery(&user).Where("uuid", receiveId).First(ctx, &user); err != nil {
			log.Println(err)
			return constant.SystemError, "", -1
		}
		session.ReceiveName = user.Nickname
		session.Avatar = user.Avatar
	} else {
		var group model.GroupInfo
		if err := dao.ActiveQuery(&group).Where("uuid", receiveId).First(ctx, &group); err != nil {
			log.Println(err)
			return constant.SystemError, "", -1
		}
		session.ReceiveName = group.Name
		session.Avatar = group.Avatar
	}

	if _, err := dao.Engine.Model(&session).Insert(ctx, &session); err != nil {
		log.Println(err)
		return constant.SystemError, "", -1
	}
	_ = dao.SessionListCache.Delete(ctx, sendId)
	return "session created", session.Uuid, 0
}

// loadSessions returns the caller's active sessions, served through the
// read-through session cache.
func loadSessions(ctx context.Context, ownerId string) ([]model.Session, error) {
	if view, err := dao.SessionListCache.Get(ctx, ownerId); err == nil {
		var sessions []model.Session
		if err := json.Unmarshal(view.ByteSlice(), &sessions); err == nil {
			return sessions, nil
		}
	}
	var sessions []model.Session
	err := dao.ActiveQuery(&sessions).
		Where("send_id", ownerId).
		OrderBy("created_at", "desc").
		Find(ctx, &sessions)
	return sessions, err
}

func DeleteSession(ctx context.Context, ownerId, sessionId string) (string, int) {
	var target model.Session
	if err := dao.Engine.Model(&target).Where("uuid", sessionId).First(ctx, &target); err == nil && IsSwiftx(target.ReceiveId) {
		return "the Swiftx session cannot be deleted", -2
	}
	now := time.Now()
	_, err := dao.Engine.Model(&model.Session{}).Where("uuid", sessionId).Update(ctx, bson.M{"deleted_at": now})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	_ = dao.SessionListCache.Delete(ctx, ownerId)
	return "deleted", 0
}

// MarkSessionRead advances the owner's read cursor for the conversation so
// its unread count drops to zero.
func MarkSessionRead(ctx context.Context, ownerId, receiveId string) (string, int) {
	if ownerId == "" || receiveId == "" {
		return "owner_id and receive_id are required", -2
	}
	if _, err := dao.Engine.Model(&model.Session{}).
		Where("send_id", ownerId).Where("receive_id", receiveId).WhereNull("deleted_at").
		Update(ctx, bson.M{"last_read_at": time.Now()}); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	_ = dao.SessionListCache.Delete(ctx, ownerId)
	return "marked as read", 0
}

// ensurePeerSession guarantees the owner has an active session pointing at
// peer, restoring a soft-deleted one or creating it from the peer's profile.
func ensurePeerSession(ctx context.Context, ownerId, peerId string) {
	var session model.Session
	err := dao.Engine.Model(&session).
		Where("send_id", ownerId).Where("receive_id", peerId).
		First(ctx, &session)
	if err == swifty_orm.ErrNotFound {
		createSession(ctx, ownerId, peerId)
		return
	}
	if err != nil {
		log.Printf("ensurePeerSession %s->%s failed: %v", ownerId, peerId, err)
		return
	}
	if session.DeletedAt != nil {
		if _, err := dao.Engine.Model(&model.Session{}).Where("uuid", session.Uuid).
			Update(ctx, bson.M{"$unset": bson.M{"deleted_at": ""}}); err != nil {
			log.Printf("ensurePeerSession restore %s failed: %v", session.Uuid, err)
			return
		}
		_ = dao.SessionListCache.Delete(ctx, ownerId)
	}
}

// touchDirectSessions makes a direct message surface in both participants'
// session lists, mirroring the legacy chat-list behavior.
func touchDirectSessions(ctx context.Context, sendId, receiveId string) {
	ensurePeerSession(ctx, sendId, receiveId)
	ensurePeerSession(ctx, receiveId, sendId)
}

// touchGroupSessions guarantees every group member has an active session for
// the group, restoring soft-deleted ones and creating missing ones in bulk.
func touchGroupSessions(ctx context.Context, group *model.GroupInfo) {
	if group == nil || len(group.Members) == 0 {
		return
	}
	var sessions []model.Session
	if err := dao.Engine.Model(&sessions).
		Where("receive_id", group.Uuid).
		WhereIn("send_id", group.Members).
		Find(ctx, &sessions); err != nil {
		log.Printf("touchGroupSessions %s failed: %v", group.Uuid, err)
		return
	}
	existing := make(map[string]*model.Session, len(sessions))
	for i := range sessions {
		existing[sessions[i].SendId] = &sessions[i]
	}

	now := time.Now()
	var restoreUuids []string
	var created []any
	var affected []string
	for _, member := range group.Members {
		s, ok := existing[member]
		switch {
		case !ok:
			created = append(created, &model.Session{
				Uuid:        "S" + util.GetNowAndLenRandomString(11),
				SendId:      member,
				ReceiveId:   group.Uuid,
				ReceiveName: group.Name,
				Avatar:      group.Avatar,
				CreatedAt:   now,
			})
			affected = append(affected, member)
		case s.DeletedAt != nil:
			restoreUuids = append(restoreUuids, s.Uuid)
			affected = append(affected, member)
		}
	}
	if len(restoreUuids) > 0 {
		if _, err := dao.Engine.Model(&model.Session{}).WhereIn("uuid", restoreUuids).
			Update(ctx, bson.M{"$unset": bson.M{"deleted_at": ""}}); err != nil {
			log.Printf("touchGroupSessions restore failed: %v", err)
		}
	}
	if len(created) > 0 {
		if _, err := dao.Engine.Model(&model.Session{}).Insert(ctx, created...); err != nil {
			log.Printf("touchGroupSessions insert failed: %v", err)
		}
	}
	for _, member := range affected {
		_ = dao.SessionListCache.Delete(ctx, member)
	}
}

func CheckOpenSessionAllowed(ctx context.Context, sendId, receiveId string) (string, bool, int) {
	if sendId == "" || receiveId == "" {
		return "send_id and receive_id are required", false, -2
	}
	var contact model.UserContact
	err := dao.ActiveQuery(&contact).Where("user_id", sendId).Where("contact_id", receiveId).First(ctx, &contact)
	if err != nil {
		log.Println(err)
		return constant.SystemError, false, -1
	}
	if contact.Status == constant.ContactBeBlack {
		return "blocked by the other user", false, -2
	}
	if contact.Status == constant.ContactBlack {
		return "unblock the user first", false, -2
	}
	if receiveId[0] == 'U' {
		var user model.UserInfo
		if err := dao.ActiveQuery(&user).Where("uuid", receiveId).First(ctx, &user); err != nil {
			return constant.SystemError, false, -1
		}
		if user.Status == constant.UserStatusDisable {
			return "target user is disabled", false, -2
		}
	} else {
		var group model.GroupInfo
		if err := dao.ActiveQuery(&group).Where("uuid", receiveId).First(ctx, &group); err != nil {
			return constant.SystemError, false, -1
		}
		if group.Status == constant.GroupStatusDisable {
			return "target group is disabled", false, -2
		}
	}
	return "allowed", true, 0
}
