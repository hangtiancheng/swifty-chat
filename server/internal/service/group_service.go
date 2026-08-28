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
	"log"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/hangtiancheng/swifty-chat/server/internal/constant"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"github.com/hangtiancheng/swifty-chat/server/internal/util"

	"github.com/hangtiancheng/swifty.go/swifty_orm"
)

type GroupInfoResponse struct {
	Uuid      string   `json:"uuid"`
	Name      string   `json:"name"`
	Notice    string   `json:"notice"`
	Members   []string `json:"members"`
	MemberCnt int      `json:"member_cnt"`
	OwnerId   string   `json:"owner_id"`
	AddMode   int8     `json:"add_mode"`
	Avatar    string   `json:"avatar"`
	Status    int8     `json:"status"`
}

type GroupListItem struct {
	GroupId   string `json:"group_id"`
	Name      string `json:"name"`
	MemberCnt int    `json:"member_cnt"`
	OwnerId   string `json:"owner_id"`
	Avatar    string `json:"avatar"`
}

type AdminGroupItem struct {
	GroupId   string `json:"group_id"`
	Name      string `json:"name"`
	MemberCnt int    `json:"member_cnt"`
	OwnerId   string `json:"owner_id"`
	Avatar    string `json:"avatar"`
	Status    int8   `json:"status"`
	IsDeleted bool   `json:"is_deleted"`
}

func CreateGroup(ctx context.Context, name, ownerId, avatar, notice string, addMode int8, memberIds []string) (string, *GroupInfoResponse, int) {
	if addMode != constant.GroupAddModeDirect && addMode != constant.GroupAddModeReview {
		return "invalid add_mode", nil, -2
	}

	// Only existing, active users besides the owner become initial members.
	members := []string{ownerId}
	if len(memberIds) > 0 {
		var users []model.UserInfo
		if err := dao.ActiveQuery(&users).WhereIn("uuid", memberIds).Find(ctx, &users); err != nil {
			log.Println(err)
			return constant.SystemError, nil, -1
		}
		for _, u := range users {
			if u.Uuid != ownerId && !slices.Contains(members, u.Uuid) {
				members = append(members, u.Uuid)
			}
		}
	}

	group := model.GroupInfo{
		Uuid:      "G" + util.GetNowAndLenRandomString(11),
		Name:      name,
		Notice:    notice,
		Members:   members,
		MemberCnt: len(members),
		OwnerId:   ownerId,
		AddMode:   addMode,
		Avatar:    avatar,
		Status:    constant.GroupStatusNormal,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if group.Avatar == "" {
		group.Avatar = "https://vitejs.dev/logo.svg"
	}

	err := dao.WithTransaction(ctx, func(sc context.Context, e *swifty_orm.Engine) error {
		if _, err := e.Model(&group).Insert(sc, &group); err != nil {
			return err
		}
		for _, member := range members {
			contact := model.UserContact{
				UserId:      member,
				ContactId:   group.Uuid,
				ContactType: constant.ContactTypeGroup,
				Status:      constant.ContactNormal,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if _, err := e.Model(&contact).Insert(sc, &contact); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("CreateGroup failed: %v", err)
		return constant.SystemError, nil, -1
	}

	// A welcome message keeps the fresh group visible in session previews,
	// like the legacy backend's "欢迎" seed message.
	welcome := model.Message{
		Uuid:      "M" + util.GetNowAndLenRandomString(11),
		Type:      constant.MessageText,
		Content:   "Welcome to " + group.Name + "!",
		SendId:    ownerId,
		ReceiveId: group.Uuid,
		Status:    constant.MessageSent,
		CreatedAt: time.Now(),
	}
	var owner model.UserInfo
	if err := dao.ActiveQuery(&owner).Where("uuid", ownerId).First(ctx, &owner); err == nil {
		welcome.SendName = owner.Nickname
		welcome.SendAvatar = owner.Avatar
	}
	if _, err := dao.Engine.Model(&welcome).Insert(ctx, &welcome); err != nil {
		log.Printf("CreateGroup: welcome message failed: %v", err)
	}
	touchGroupSessions(ctx, &group)
	for _, member := range members {
		if member != ownerId {
			PushSystem(constant.NotifyGroup, member)
		}
		PushSystem(constant.NotifySession, member)
	}

	return "group created", toGroupInfoResponse(&group), 0
}

func GetGroupInfo(ctx context.Context, uuid string) (string, *GroupInfoResponse, int) {
	var group model.GroupInfo
	err := dao.ActiveQuery(&group).Where("uuid", uuid).First(ctx, &group)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	return "group info retrieved", toGroupInfoResponse(&group), 0
}

func LoadMyGroup(ctx context.Context, ownerId string) (string, []GroupListItem, int) {
	var groups []model.GroupInfo
	err := dao.ActiveQuery(&groups).Where("owner_id", ownerId).Find(ctx, &groups)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []GroupListItem
	for _, g := range groups {
		list = append(list, GroupListItem{
			GroupId: g.Uuid, Name: g.Name, MemberCnt: g.MemberCnt, OwnerId: g.OwnerId, Avatar: g.Avatar,
		})
	}
	return "success", list, 0
}

func CheckGroupAddMode(ctx context.Context, groupId string) (string, int8, int) {
	var group model.GroupInfo
	err := dao.ActiveQuery(&group).Where("uuid", groupId).First(ctx, &group)
	if err != nil {
		log.Println(err)
		return constant.SystemError, 0, -1
	}
	return "success", group.AddMode, 0
}

// addGroupMember atomically appends the user to the members array via
// $addToSet; member_cnt is only incremented when the set actually grew.
func addGroupMember(ctx context.Context, e *swifty_orm.Engine, groupId, userId string) error {
	res, err := e.Model(&model.GroupInfo{}).Where("uuid", groupId).
		Upsert(ctx, bson.M{"$addToSet": bson.M{"members": userId}})
	if err != nil {
		return err
	}
	if res.ModifiedCount > 0 {
		if _, err := e.Model(&model.GroupInfo{}).Where("uuid", groupId).Update(ctx, bson.M{
			"$inc": bson.M{"member_cnt": 1},
			"$set": bson.M{"updated_at": time.Now()},
		}); err != nil {
			return err
		}
	}
	return nil
}

// removeGroupMember atomically pulls the user from the members array;
// member_cnt is only decremented when the member was actually present.
func removeGroupMember(ctx context.Context, e *swifty_orm.Engine, groupId, userId string) error {
	res, err := e.Model(&model.GroupInfo{}).Where("uuid", groupId).
		Upsert(ctx, bson.M{"$pull": bson.M{"members": userId}})
	if err != nil {
		return err
	}
	if res.ModifiedCount > 0 {
		if _, err := e.Model(&model.GroupInfo{}).Where("uuid", groupId).Update(ctx, bson.M{
			"$inc": bson.M{"member_cnt": -1},
			"$set": bson.M{"updated_at": time.Now()},
		}); err != nil {
			return err
		}
	}
	return nil
}

// ensureGroupContact inserts the user→group contact, restoring a previously
// soft-deleted record instead of inserting a duplicate.
func ensureGroupContact(ctx context.Context, e *swifty_orm.Engine, userId, groupId string) error {
	now := time.Now()
	var existing model.UserContact
	err := e.Model(&existing).
		Where("user_id", userId).Where("contact_id", groupId).
		First(ctx, &existing)
	if err == nil {
		_, err = e.Model(&model.UserContact{}).
			Where("user_id", userId).Where("contact_id", groupId).
			Update(ctx, bson.M{
				"$set":   bson.M{"status": constant.ContactNormal, "updated_at": now},
				"$unset": bson.M{"deleted_at": ""},
			})
		return err
	}
	if err != swifty_orm.ErrNotFound {
		return err
	}
	contact := model.UserContact{
		UserId: userId, ContactId: groupId,
		ContactType: constant.ContactTypeGroup, Status: constant.ContactNormal,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err = e.Model(&contact).Insert(ctx, &contact)
	return err
}

func EnterGroupDirectly(ctx context.Context, userId, groupId string) (string, int) {
	var group model.GroupInfo
	err := dao.ActiveQuery(&group).Where("uuid", groupId).First(ctx, &group)
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if group.Status == constant.GroupStatusDisable {
		return "group is disabled", -2
	}
	if group.AddMode != constant.GroupAddModeDirect {
		return "group requires owner approval", -2
	}
	if slices.Contains(group.Members, userId) {
		return "already a group member", -2
	}

	if err := addGroupMember(ctx, dao.Engine, groupId, userId); err != nil {
		log.Printf("EnterGroupDirectly: add member failed: %v", err)
		return constant.SystemError, -1
	}
	if err := ensureGroupContact(ctx, dao.Engine, userId, groupId); err != nil {
		log.Printf("EnterGroupDirectly: insert contact failed: %v", err)
		return constant.SystemError, -1
	}
	return "joined group", 0
}

// InviteGroupMembers adds the given users to the group, skipping the ones
// already in it — migrated from the legacy "invite friends to group" flow.
func InviteGroupMembers(ctx context.Context, groupId string, memberIds []string) (string, int) {
	var group model.GroupInfo
	err := dao.ActiveQuery(&group).Where("uuid", groupId).First(ctx, &group)
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if group.Status == constant.GroupStatusDisable {
		return "group is disabled", -2
	}

	var candidates []string
	for _, id := range memberIds {
		if !slices.Contains(group.Members, id) && !slices.Contains(candidates, id) {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return "all selected users are already group members", -2
	}
	var users []model.UserInfo
	if err := dao.ActiveQuery(&users).WhereIn("uuid", candidates).Find(ctx, &users); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	var newcomers []string
	for _, u := range users {
		newcomers = append(newcomers, u.Uuid)
	}
	if len(newcomers) == 0 {
		return "no valid users to invite", -2
	}

	for _, id := range newcomers {
		if err := addGroupMember(ctx, dao.Engine, groupId, id); err != nil {
			log.Printf("InviteGroupMembers: add %s failed: %v", id, err)
			return constant.SystemError, -1
		}
		if err := ensureGroupContact(ctx, dao.Engine, id, groupId); err != nil {
			log.Printf("InviteGroupMembers: contact for %s failed: %v", id, err)
			return constant.SystemError, -1
		}
	}
	// Refresh the members array before creating sessions for everyone.
	var updated model.GroupInfo
	if err := dao.ActiveQuery(&updated).Where("uuid", groupId).First(ctx, &updated); err == nil {
		touchGroupSessions(ctx, &updated)
	}
	PushSystem(constant.NotifyGroup, newcomers...)
	PushSystem(constant.NotifySession, newcomers...)
	return "members invited", 0
}

type SearchGroupItem struct {
	GroupId   string `json:"group_id"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	MemberCnt int    `json:"member_cnt"`
	AddMode   int8   `json:"add_mode"`
	IsJoined  bool   `json:"is_joined"`
}

// SearchGroups finds groups by name keyword, flagging the ones the caller has
// already joined.
func SearchGroups(ctx context.Context, ownerId, keyword string) (string, []SearchGroupItem, int) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "keyword is required", nil, -2
	}
	pattern := primitive.Regex{Pattern: regexp.QuoteMeta(keyword), Options: "i"}
	var groups []model.GroupInfo
	err := dao.ActiveQuery(&groups).
		Where("status", constant.GroupStatusNormal).
		Where(bson.M{"name": pattern}).
		Limit(20).
		Find(ctx, &groups)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []SearchGroupItem
	for _, g := range groups {
		list = append(list, SearchGroupItem{
			GroupId: g.Uuid, Name: g.Name, Avatar: g.Avatar,
			MemberCnt: g.MemberCnt, AddMode: g.AddMode,
			IsJoined: slices.Contains(g.Members, ownerId),
		})
	}
	return "success", list, 0
}

// cleanupGroupMembership soft-deletes the member's session, contact record
// (stamped with the given status) and pending applies for the group.
func cleanupGroupMembership(ctx context.Context, e *swifty_orm.Engine, userId, groupId string, contactStatus int8) {
	now := time.Now()
	if _, err := e.Model(&model.UserContact{}).
		Where("user_id", userId).Where("contact_id", groupId).WhereNull("deleted_at").
		Update(ctx, bson.M{"status": contactStatus, "deleted_at": now}); err != nil {
		log.Printf("cleanupGroupMembership: contact update failed: %v", err)
	}
	if _, err := e.Model(&model.Session{}).
		Where("send_id", userId).Where("receive_id", groupId).WhereNull("deleted_at").
		Update(ctx, bson.M{"deleted_at": now}); err != nil {
		log.Printf("cleanupGroupMembership: session update failed: %v", err)
	}
	if _, err := e.Model(&model.ContactApply{}).
		Where("user_id", userId).Where("contact_id", groupId).WhereNull("deleted_at").
		Update(ctx, bson.M{"deleted_at": now}); err != nil {
		log.Printf("cleanupGroupMembership: apply update failed: %v", err)
	}
	_ = dao.SessionListCache.Delete(ctx, userId)
}

func LeaveGroup(ctx context.Context, userId, groupId string) (string, int) {
	var group model.GroupInfo
	err := dao.ActiveQuery(&group).Where("uuid", groupId).First(ctx, &group)
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if group.OwnerId == userId {
		return "owner cannot leave the group, dismiss it instead", -2
	}
	if err := removeGroupMember(ctx, dao.Engine, groupId, userId); err != nil {
		log.Printf("LeaveGroup: remove member failed: %v", err)
		return constant.SystemError, -1
	}
	cleanupGroupMembership(ctx, dao.Engine, userId, groupId, constant.ContactQuit)
	return "left group", 0
}

// cascadeGroupRemoval soft-deletes every session, contact and apply that
// references the group.
func cascadeGroupRemoval(ctx context.Context, e *swifty_orm.Engine, groupId string) error {
	now := time.Now()
	invalidateSessionCacheByReceiver(ctx, groupId)
	if _, err := e.Model(&model.Session{}).
		Where("receive_id", groupId).WhereNull("deleted_at").
		Update(ctx, bson.M{"deleted_at": now}); err != nil {
		return err
	}
	if _, err := e.Model(&model.UserContact{}).
		Where("contact_id", groupId).WhereNull("deleted_at").
		Update(ctx, bson.M{"deleted_at": now}); err != nil {
		return err
	}
	if _, err := e.Model(&model.ContactApply{}).
		Where("contact_id", groupId).WhereNull("deleted_at").
		Update(ctx, bson.M{"deleted_at": now}); err != nil {
		return err
	}
	return nil
}

func DismissGroup(ctx context.Context, groupId string) (string, int) {
	var group model.GroupInfo
	if err := dao.ActiveQuery(&group).Where("uuid", groupId).First(ctx, &group); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	err := dao.WithTransaction(ctx, func(sc context.Context, e *swifty_orm.Engine) error {
		if _, err := e.Model(&model.GroupInfo{}).Where("uuid", groupId).Update(sc, bson.M{
			"status": constant.GroupStatusDismiss, "deleted_at": time.Now(),
		}); err != nil {
			return err
		}
		return cascadeGroupRemoval(sc, e, groupId)
	})
	if err != nil {
		log.Printf("DismissGroup %s failed: %v", groupId, err)
		return constant.SystemError, -1
	}
	PushSystem(constant.NotifyGroup, group.Members...)
	PushSystem(constant.NotifySession, group.Members...)
	return "group dismissed", 0
}

func UpdateGroupInfo(ctx context.Context, uuid string, fields bson.M) (string, int) {
	if len(fields) == 0 {
		return "group info updated", 0
	}
	fields["updated_at"] = time.Now()
	_, err := dao.Engine.Model(&model.GroupInfo{}).Where("uuid", uuid).Update(ctx, fields)
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}

	// Keep the denormalized session fields in sync with the group profile.
	sessionFields := bson.M{}
	if name, ok := fields["name"]; ok {
		sessionFields["receive_name"] = name
	}
	if avatar, ok := fields["avatar"]; ok {
		sessionFields["avatar"] = avatar
	}
	if len(sessionFields) > 0 {
		if _, err := dao.Engine.Model(&model.Session{}).
			Where("receive_id", uuid).WhereNull("deleted_at").
			Update(ctx, sessionFields); err != nil {
			log.Printf("UpdateGroupInfo: session sync failed: %v", err)
		}
		invalidateSessionCacheByReceiver(ctx, uuid)
	}
	return "group info updated", 0
}

type GroupMemberItem struct {
	UserId        string `json:"user_id"`
	Uuid          string `json:"uuid"`
	Nickname      string `json:"nickname"`
	Avatar        string `json:"avatar"`
	IsOwner       bool   `json:"is_owner"`
	JoinedAt      string `json:"joined_at"`
	LastMessageAt string `json:"last_message_at"`
}

func GetGroupMemberList(ctx context.Context, groupId string) (string, []GroupMemberItem, int) {
	var group model.GroupInfo
	err := dao.ActiveQuery(&group).Where("uuid", groupId).First(ctx, &group)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var users []model.UserInfo
	err = dao.ActiveQuery(&users).WhereIn("uuid", group.Members).Find(ctx, &users)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}

	// Join time comes from the member's group contact record; the owner's is
	// the group creation time.
	joinedAt := make(map[string]time.Time, len(group.Members))
	var contacts []model.UserContact
	if err := dao.ActiveQuery(&contacts).
		Where("contact_id", groupId).
		WhereIn("user_id", group.Members).
		Find(ctx, &contacts); err == nil {
		for _, c := range contacts {
			joinedAt[c.UserId] = c.CreatedAt
		}
	}
	joinedAt[group.OwnerId] = group.CreatedAt

	// Last-speak time per member via a grouped max over the group's messages.
	lastSpoke := make(map[string]time.Time, len(group.Members))
	var rows []struct {
		SendId string    `bson:"send_id"`
		LastAt time.Time `bson:"last_at"`
	}
	if err := dao.Engine.Model(&model.Message{}).
		Where("receive_id", groupId).
		Where("type", "!=", constant.MessageAudioOrVideo).
		WhereIn("send_id", group.Members).
		GroupBy("send_id").
		MaxAs("created_at", "last_at").
		Aggregate(ctx, &rows); err == nil {
		for _, r := range rows {
			lastSpoke[r.SendId] = r.LastAt
		}
	} else {
		log.Printf("GetGroupMemberList: last message aggregate failed: %v", err)
	}

	const timeLayout = "2006-01-02 15:04:05"
	var list []GroupMemberItem
	for _, u := range users {
		item := GroupMemberItem{
			UserId: u.Uuid, Uuid: u.Uuid, Nickname: u.Nickname, Avatar: u.Avatar,
			IsOwner: u.Uuid == group.OwnerId,
		}
		if t, ok := joinedAt[u.Uuid]; ok {
			item.JoinedAt = t.Format(timeLayout)
		}
		if t, ok := lastSpoke[u.Uuid]; ok {
			item.LastMessageAt = t.Format(timeLayout)
		}
		list = append(list, item)
	}
	return "success", list, 0
}

func RemoveGroupMembers(ctx context.Context, groupId string, memberIds []string) (string, int) {
	var group model.GroupInfo
	err := dao.ActiveQuery(&group).Where("uuid", groupId).First(ctx, &group)
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if slices.Contains(memberIds, group.OwnerId) {
		return "cannot remove the group owner", -2
	}

	if _, err := dao.Engine.Model(&model.GroupInfo{}).Where("uuid", groupId).Update(ctx, bson.M{
		"$pull": bson.M{"members": bson.M{"$in": memberIds}},
		"$set":  bson.M{"updated_at": time.Now()},
	}); err != nil {
		log.Printf("RemoveGroupMembers: pull failed: %v", err)
		return constant.SystemError, -1
	}
	// Recompute member_cnt from the authoritative members array.
	var updated model.GroupInfo
	if err := dao.ActiveQuery(&updated).Where("uuid", groupId).First(ctx, &updated); err == nil {
		if _, err := dao.Engine.Model(&model.GroupInfo{}).Where("uuid", groupId).
			Update(ctx, bson.M{"member_cnt": len(updated.Members)}); err != nil {
			log.Printf("RemoveGroupMembers: member_cnt update failed: %v", err)
		}
	}

	for _, id := range memberIds {
		cleanupGroupMembership(ctx, dao.Engine, id, groupId, constant.ContactKicked)
	}
	PushSystem(constant.NotifyGroup, memberIds...)
	PushSystem(constant.NotifySession, memberIds...)
	return "members removed", 0
}

func GetGroupInfoList(ctx context.Context) (string, []AdminGroupItem, int) {
	var groups []model.GroupInfo
	err := dao.Engine.Model(&groups).Find(ctx, &groups)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []AdminGroupItem
	for _, g := range groups {
		list = append(list, AdminGroupItem{
			GroupId: g.Uuid, Name: g.Name, MemberCnt: g.MemberCnt, OwnerId: g.OwnerId,
			Avatar: g.Avatar, Status: g.Status, IsDeleted: g.DeletedAt != nil,
		})
	}
	return "success", list, 0
}

func DeleteGroups(ctx context.Context, uuidList []string) (string, int) {
	now := time.Now()
	_, err := dao.Engine.Model(&model.GroupInfo{}).WhereIn("uuid", uuidList).Update(ctx, bson.M{"deleted_at": now})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	for _, uuid := range uuidList {
		if err := cascadeGroupRemoval(ctx, dao.Engine, uuid); err != nil {
			log.Printf("DeleteGroups: cascade for %s failed: %v", uuid, err)
		}
	}
	return "groups deleted", 0
}

func SetGroupsStatus(ctx context.Context, uuidList []string, status int8) (string, int) {
	_, err := dao.Engine.Model(&model.GroupInfo{}).WhereIn("uuid", uuidList).Update(ctx, bson.M{"status": status, "updated_at": time.Now()})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if status == constant.GroupStatusDisable {
		now := time.Now()
		for _, uuid := range uuidList {
			invalidateSessionCacheByReceiver(ctx, uuid)
			if _, err := dao.Engine.Model(&model.Session{}).
				Where("receive_id", uuid).WhereNull("deleted_at").
				Update(ctx, bson.M{"deleted_at": now}); err != nil {
				log.Printf("SetGroupsStatus: session cleanup for %s failed: %v", uuid, err)
			}
		}
	}
	return "group status updated", 0
}

func toGroupInfoResponse(g *model.GroupInfo) *GroupInfoResponse {
	return &GroupInfoResponse{
		Uuid: g.Uuid, Name: g.Name, Notice: g.Notice, Members: g.Members,
		MemberCnt: g.MemberCnt, OwnerId: g.OwnerId, AddMode: g.AddMode,
		Avatar: g.Avatar, Status: g.Status,
	}
}
