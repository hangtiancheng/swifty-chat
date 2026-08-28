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
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/hangtiancheng/swifty-chat/server/internal/constant"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"github.com/hangtiancheng/swifty-chat/server/internal/util"

	"github.com/hangtiancheng/swifty.go/swifty_orm"
)

type ContactInfoResponse struct {
	ContactId        string   `json:"contact_id"`
	ContactName      string   `json:"contact_name"`
	ContactAvatar    string   `json:"contact_avatar"`
	ContactPhone     string   `json:"contact_phone"`
	ContactEmail     string   `json:"contact_email"`
	ContactGender    int8     `json:"contact_gender"`
	ContactSignature string   `json:"contact_signature"`
	ContactBirthday  string   `json:"contact_birthday"`
	ContactNotice    string   `json:"contact_notice"`
	ContactMembers   []string `json:"contact_members"`
	ContactMemberCnt int      `json:"contact_member_cnt"`
	ContactOwnerId   string   `json:"contact_owner_id"`
	ContactAddMode   int8     `json:"contact_add_mode"`
}

type ContactApplyResponse struct {
	ApplyId     string `json:"apply_id"`
	UserId      string `json:"user_id"`
	ContactId   string `json:"contact_id"`
	ContactName string `json:"contact_name"`
	ContactType int8   `json:"contact_type"`
	Status      int8   `json:"status"`
	Message     string `json:"message"`
}

type ContactListItem struct {
	UserId   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Status   int8   `json:"status"`
	NoteName string `json:"note_name"`
	TagId    string `json:"tag_id"`
	Online   bool   `json:"online"`
}

type TagItem struct {
	TagId string `json:"tag_id"`
	Name  string `json:"name"`
}

// GetTagList returns the caller's contact tags in creation order.
func GetTagList(ctx context.Context, ownerId string) (string, []TagItem, int) {
	var tags []model.ContactTag
	err := dao.ActiveQuery(&tags).Where("user_id", ownerId).OrderBy("created_at").Find(ctx, &tags)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []TagItem
	for _, t := range tags {
		list = append(list, TagItem{TagId: t.Uuid, Name: t.Name})
	}
	return "success", list, 0
}

func AddTag(ctx context.Context, ownerId, name string) (string, *TagItem, int) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tag name is required", nil, -2
	}
	var existing model.ContactTag
	if err := dao.ActiveQuery(&existing).Where("user_id", ownerId).Where("name", name).First(ctx, &existing); err == nil {
		return "tag already exists", nil, -2
	}
	tag := model.ContactTag{
		Uuid:      "T" + util.GetNowAndLenRandomString(11),
		UserId:    ownerId,
		Name:      name,
		CreatedAt: time.Now(),
	}
	if _, err := dao.Engine.Model(&tag).Insert(ctx, &tag); err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	return "tag created", &TagItem{TagId: tag.Uuid, Name: tag.Name}, 0
}

// UpdateContact edits the note name and/or tag of one of the caller's
// contacts. Nil pointers leave the field untouched.
func UpdateContact(ctx context.Context, userId, contactId string, noteName, tagId *string) (string, int) {
	fields := bson.M{"updated_at": time.Now()}
	if noteName != nil {
		fields["note_name"] = *noteName
	}
	if tagId != nil {
		fields["tag_id"] = *tagId
	}
	if len(fields) == 1 {
		return "nothing to update", -2
	}
	if _, err := dao.Engine.Model(&model.UserContact{}).
		Where("user_id", userId).Where("contact_id", contactId).WhereNull("deleted_at").
		Update(ctx, fields); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	return "contact updated", 0
}

// userNamesByUuid batch-loads nicknames for the given user ids.
func userNamesByUuid(ctx context.Context, uuids []string) map[string]string {
	names := make(map[string]string, len(uuids))
	if len(uuids) == 0 {
		return names
	}
	var users []model.UserInfo
	if err := dao.ActiveQuery(&users).WhereIn("uuid", uuids).Find(ctx, &users); err != nil {
		log.Printf("userNamesByUuid failed: %v", err)
		return names
	}
	for _, u := range users {
		names[u.Uuid] = u.Nickname
	}
	return names
}

// GetUserList returns the caller's user contacts. Blocked contacts are
// included (with their status) so the client can offer an unblock action.
func GetUserList(ctx context.Context, ownerId string) (string, []ContactListItem, int) {
	var contacts []model.UserContact
	err := dao.ActiveQuery(&contacts).
		Where("user_id", ownerId).
		Where("contact_type", constant.ContactTypeUser).
		WhereIn("status", []int8{constant.ContactNormal, constant.ContactBlack, constant.ContactBeBlack}).
		Find(ctx, &contacts)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}

	ids := make([]string, 0, len(contacts))
	contactById := make(map[string]*model.UserContact, len(contacts))
	for i := range contacts {
		ids = append(ids, contacts[i].ContactId)
		contactById[contacts[i].ContactId] = &contacts[i]
	}
	if len(ids) == 0 {
		return "success", nil, 0
	}
	var users []model.UserInfo
	if err := dao.ActiveQuery(&users).WhereIn("uuid", ids).Find(ctx, &users); err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []ContactListItem
	for _, user := range users {
		c := contactById[user.Uuid]
		if c == nil {
			continue
		}
		list = append(list, ContactListItem{
			UserId: user.Uuid, Nickname: user.Nickname, Avatar: user.Avatar,
			Status: c.Status, NoteName: c.NoteName, TagId: c.TagId,
			Online: IsOnline(user.Uuid),
		})
	}
	return "success", list, 0
}

func LoadMyJoinedGroup(ctx context.Context, ownerId string) (string, []GroupListItem, int) {
	var contacts []model.UserContact
	err := dao.ActiveQuery(&contacts).
		Where("user_id", ownerId).
		Where("contact_type", constant.ContactTypeGroup).
		Where("status", constant.ContactNormal).
		Find(ctx, &contacts)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	ids := make([]string, 0, len(contacts))
	for _, c := range contacts {
		ids = append(ids, c.ContactId)
	}
	if len(ids) == 0 {
		return "success", nil, 0
	}
	var groups []model.GroupInfo
	if err := dao.ActiveQuery(&groups).WhereIn("uuid", ids).Find(ctx, &groups); err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []GroupListItem
	for _, group := range groups {
		// Groups the caller owns are listed under "My Groups" instead.
		if group.OwnerId == ownerId {
			continue
		}
		list = append(list, GroupListItem{
			GroupId: group.Uuid, Name: group.Name, MemberCnt: group.MemberCnt,
			OwnerId: group.OwnerId, Avatar: group.Avatar,
		})
	}
	return "success", list, 0
}

func GetContactInfo(ctx context.Context, userId, contactId string) (string, *ContactInfoResponse, int) {
	if len(contactId) > 0 && contactId[0] == 'G' {
		var group model.GroupInfo
		if err := dao.ActiveQuery(&group).Where("uuid", contactId).First(ctx, &group); err != nil {
			log.Println(err)
			return constant.SystemError, nil, -1
		}
		return "success", &ContactInfoResponse{
			ContactId:        group.Uuid,
			ContactName:      group.Name,
			ContactAvatar:    group.Avatar,
			ContactNotice:    group.Notice,
			ContactMembers:   group.Members,
			ContactMemberCnt: group.MemberCnt,
			ContactOwnerId:   group.OwnerId,
			ContactAddMode:   group.AddMode,
		}, 0
	}
	var user model.UserInfo
	if err := dao.ActiveQuery(&user).Where("uuid", contactId).First(ctx, &user); err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	return "success", &ContactInfoResponse{
		ContactId:        user.Uuid,
		ContactName:      user.Nickname,
		ContactAvatar:    user.Avatar,
		ContactPhone:     user.Telephone,
		ContactEmail:     user.Email,
		ContactGender:    user.Gender,
		ContactSignature: user.Signature,
		ContactBirthday:  user.Birthday,
	}, 0
}

func ApplyContact(ctx context.Context, userId, contactId string, contactType int8, message string) (string, int) {
	// Validate the target and refuse disabled targets.
	if contactType == constant.ContactTypeUser {
		var target model.UserInfo
		if err := dao.ActiveQuery(&target).Where("uuid", contactId).First(ctx, &target); err != nil {
			if err == swifty_orm.ErrNotFound {
				return "user not found", -2
			}
			log.Println(err)
			return constant.SystemError, -1
		}
		if target.Status == constant.UserStatusDisable {
			return "user is disabled", -2
		}
	} else {
		var target model.GroupInfo
		if err := dao.ActiveQuery(&target).Where("uuid", contactId).First(ctx, &target); err != nil {
			if err == swifty_orm.ErrNotFound {
				return "group not found", -2
			}
			log.Println(err)
			return constant.SystemError, -1
		}
		if target.Status == constant.GroupStatusDisable {
			return "group is disabled", -2
		}
	}

	// Re-applying updates the existing record instead of inserting a new one;
	// a blacklisted apply blocks any further attempts.
	var existing model.ContactApply
	err := dao.ActiveQuery(&existing).
		Where("user_id", userId).Where("contact_id", contactId).
		First(ctx, &existing)
	if err == nil {
		if existing.Status == constant.ApplyStatusBlack {
			return "you have been blocked by the recipient", -2
		}
		if _, err := dao.Engine.Model(&model.ContactApply{}).Where("uuid", existing.Uuid).Update(ctx, bson.M{
			"status": constant.ApplyStatusApplying, "message": message, "last_apply_at": time.Now(),
		}); err != nil {
			log.Println(err)
			return constant.SystemError, -1
		}
		notifyApplyRecipient(ctx, contactId, contactType)
		return "application submitted", 0
	}
	if err != swifty_orm.ErrNotFound {
		log.Println(err)
		return constant.SystemError, -1
	}

	apply := model.ContactApply{
		Uuid:        "A" + util.GetNowAndLenRandomString(11),
		UserId:      userId,
		ContactId:   contactId,
		ContactType: contactType,
		Status:      constant.ApplyStatusApplying,
		Message:     message,
		LastApplyAt: time.Now(),
	}
	if _, err := dao.Engine.Model(&apply).Insert(ctx, &apply); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	notifyApplyRecipient(ctx, contactId, contactType)
	return "application submitted", 0
}

// notifyApplyRecipient pushes an apply notification to whoever reviews it:
// the target user for friend applies, the group owner for join applies.
func notifyApplyRecipient(ctx context.Context, contactId string, contactType int8) {
	if contactType == constant.ContactTypeUser {
		PushSystem(constant.NotifyApply, contactId)
		return
	}
	var group model.GroupInfo
	if err := dao.ActiveQuery(&group).Where("uuid", contactId).First(ctx, &group); err == nil {
		PushSystem(constant.NotifyApply, group.OwnerId)
	}
}

// GetNewContactList returns pending friend applications addressed to the user.
func GetNewContactList(ctx context.Context, userId string) (string, []ContactApplyResponse, int) {
	var applies []model.ContactApply
	err := dao.ActiveQuery(&applies).
		Where("contact_id", userId).
		Where("contact_type", constant.ContactTypeUser).
		Where("status", constant.ApplyStatusApplying).
		OrderBy("last_apply_at", "desc").
		Find(ctx, &applies)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	return "success", toApplyResponses(ctx, applies), 0
}

func toApplyResponses(ctx context.Context, applies []model.ContactApply) []ContactApplyResponse {
	ids := make([]string, 0, len(applies))
	for _, a := range applies {
		ids = append(ids, a.UserId)
	}
	names := userNamesByUuid(ctx, ids)
	var list []ContactApplyResponse
	for _, a := range applies {
		contactName := a.UserId
		if n, ok := names[a.UserId]; ok {
			contactName = n
		}
		list = append(list, ContactApplyResponse{
			ApplyId: a.Uuid, UserId: a.UserId, ContactId: a.ContactId,
			ContactName: contactName, ContactType: a.ContactType,
			Status: a.Status, Message: a.Message,
		})
	}
	return list
}

// ensureUserContact inserts a user↔user contact, restoring a soft-deleted
// record if one exists.
func ensureUserContact(ctx context.Context, e *swifty_orm.Engine, userId, contactId string) error {
	now := time.Now()
	var existing model.UserContact
	err := e.Model(&existing).
		Where("user_id", userId).Where("contact_id", contactId).
		First(ctx, &existing)
	if err == nil {
		_, err = e.Model(&model.UserContact{}).
			Where("user_id", userId).Where("contact_id", contactId).
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
		UserId: userId, ContactId: contactId,
		ContactType: constant.ContactTypeUser, Status: constant.ContactNormal,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err = e.Model(&contact).Insert(ctx, &contact)
	return err
}

// PassContactApply approves an application. Friend applications create the
// two-way contact pair; group applications add the applicant to the group.
func PassContactApply(ctx context.Context, applyId string) (string, int) {
	var apply model.ContactApply
	err := dao.ActiveQuery(&apply).Where("uuid", applyId).First(ctx, &apply)
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if apply.Status != constant.ApplyStatusApplying {
		return "application already handled", -2
	}

	err = dao.WithTransaction(ctx, func(sc context.Context, e *swifty_orm.Engine) error {
		if _, err := e.Model(&model.ContactApply{}).Where("uuid", applyId).
			Update(sc, bson.M{"status": constant.ApplyStatusPass}); err != nil {
			return err
		}
		if apply.ContactType == constant.ContactTypeGroup {
			// apply.ContactId is the group id, apply.UserId the applicant.
			// Verify the group still exists: addGroupMember upserts and would
			// otherwise create a ghost group document.
			var group model.GroupInfo
			if err := e.Model(&group).WhereNull("deleted_at").Where("uuid", apply.ContactId).First(sc, &group); err != nil {
				return err
			}
			if err := addGroupMember(sc, e, apply.ContactId, apply.UserId); err != nil {
				return err
			}
			return ensureGroupContact(sc, e, apply.UserId, apply.ContactId)
		}
		if err := ensureUserContact(sc, e, apply.UserId, apply.ContactId); err != nil {
			return err
		}
		return ensureUserContact(sc, e, apply.ContactId, apply.UserId)
	})
	if err != nil {
		log.Printf("PassContactApply %s failed: %v", applyId, err)
		return constant.SystemError, -1
	}
	if apply.ContactType == constant.ContactTypeGroup {
		PushSystem(constant.NotifyGroup, apply.UserId)
		PushSystem(constant.NotifySession, apply.UserId)
	} else {
		PushSystem(constant.NotifyContact, apply.UserId, apply.ContactId)
	}
	return "application approved", 0
}

func BlackContact(ctx context.Context, userId, contactId string) (string, int) {
	now := time.Now()
	if _, err := dao.Engine.Model(&model.UserContact{}).
		Where("user_id", userId).Where("contact_id", contactId).
		Update(ctx, bson.M{"status": constant.ContactBlack, "updated_at": now}); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if _, err := dao.Engine.Model(&model.UserContact{}).
		Where("user_id", contactId).Where("contact_id", userId).
		Update(ctx, bson.M{"status": constant.ContactBeBlack, "updated_at": now}); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	// The blocker's session goes away, matching the old behavior.
	if _, err := dao.Engine.Model(&model.Session{}).
		Where("send_id", userId).Where("receive_id", contactId).WhereNull("deleted_at").
		Update(ctx, bson.M{"deleted_at": now}); err != nil {
		log.Printf("BlackContact: session cleanup failed: %v", err)
	}
	_ = dao.SessionListCache.Delete(ctx, userId)
	PushSystem(constant.NotifyContact, contactId)
	return "contact blocked", 0
}

func CancelBlackContact(ctx context.Context, userId, contactId string) (string, int) {
	now := time.Now()
	if _, err := dao.Engine.Model(&model.UserContact{}).
		Where("user_id", userId).Where("contact_id", contactId).
		Update(ctx, bson.M{"status": constant.ContactNormal, "updated_at": now}); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	if _, err := dao.Engine.Model(&model.UserContact{}).
		Where("user_id", contactId).Where("contact_id", userId).
		Update(ctx, bson.M{"status": constant.ContactNormal, "updated_at": now}); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	PushSystem(constant.NotifyContact, contactId)
	return "contact unblocked", 0
}

func DeleteContact(ctx context.Context, userId, contactId string) (string, int) {
	err := dao.WithTransaction(ctx, func(sc context.Context, e *swifty_orm.Engine) error {
		now := time.Now()
		if _, err := e.Model(&model.UserContact{}).
			Where("user_id", userId).Where("contact_id", contactId).WhereNull("deleted_at").
			Update(sc, bson.M{"status": constant.ContactDelete, "deleted_at": now}); err != nil {
			return err
		}
		if _, err := e.Model(&model.UserContact{}).
			Where("user_id", contactId).Where("contact_id", userId).WhereNull("deleted_at").
			Update(sc, bson.M{"status": constant.ContactBeDelete, "deleted_at": now}); err != nil {
			return err
		}
		// Both directions of the session and the historical applies go away,
		// so a future application starts from a clean slate.
		if _, err := e.Model(&model.Session{}).
			Where("send_id", userId).Where("receive_id", contactId).WhereNull("deleted_at").
			Update(sc, bson.M{"deleted_at": now}); err != nil {
			return err
		}
		if _, err := e.Model(&model.Session{}).
			Where("send_id", contactId).Where("receive_id", userId).WhereNull("deleted_at").
			Update(sc, bson.M{"deleted_at": now}); err != nil {
			return err
		}
		if _, err := e.Model(&model.ContactApply{}).
			Where("user_id", userId).Where("contact_id", contactId).WhereNull("deleted_at").
			Update(sc, bson.M{"deleted_at": now}); err != nil {
			return err
		}
		if _, err := e.Model(&model.ContactApply{}).
			Where("user_id", contactId).Where("contact_id", userId).WhereNull("deleted_at").
			Update(sc, bson.M{"deleted_at": now}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("DeleteContact %s->%s failed: %v", userId, contactId, err)
		return constant.SystemError, -1
	}
	_ = dao.SessionListCache.Delete(ctx, userId)
	_ = dao.SessionListCache.Delete(ctx, contactId)
	PushSystem(constant.NotifyContact, contactId)
	PushSystem(constant.NotifySession, contactId)
	return "deleted", 0
}

func RefuseContactApply(ctx context.Context, applyId string) (string, int) {
	_, err := dao.Engine.Model(&model.ContactApply{}).Where("uuid", applyId).Update(ctx, bson.M{"status": constant.ApplyStatusRefuse})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	return "application refused", 0
}

func BlackApply(ctx context.Context, applyId string) (string, int) {
	_, err := dao.Engine.Model(&model.ContactApply{}).Where("uuid", applyId).Update(ctx, bson.M{"status": constant.ApplyStatusBlack})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	return "application blocked", 0
}

// GetAddGroupList returns the pending join applications for every group the
// caller owns.
func GetAddGroupList(ctx context.Context, ownerId string) (string, []ContactApplyResponse, int) {
	var groups []model.GroupInfo
	if err := dao.ActiveQuery(&groups).Where("owner_id", ownerId).Find(ctx, &groups); err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	groupIds := make([]string, 0, len(groups))
	for _, g := range groups {
		groupIds = append(groupIds, g.Uuid)
	}
	if len(groupIds) == 0 {
		return "success", nil, 0
	}

	var applies []model.ContactApply
	err := dao.ActiveQuery(&applies).
		WhereIn("contact_id", groupIds).
		Where("contact_type", constant.ContactTypeGroup).
		Where("status", constant.ApplyStatusApplying).
		OrderBy("last_apply_at", "desc").
		Find(ctx, &applies)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	return "success", toApplyResponses(ctx, applies), 0
}
