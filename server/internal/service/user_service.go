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
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/hangtiancheng/swifty-chat/server/internal/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/constant"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"github.com/hangtiancheng/swifty-chat/server/internal/util"
)

type UserInfoResponse struct {
	Uuid      string `json:"uuid"`
	Telephone string `json:"telephone"`
	Nickname  string `json:"nickname"`
	Email     string `json:"email"`
	Avatar    string `json:"avatar"`
	Gender    int8   `json:"gender"`
	Birthday  string `json:"birthday"`
	Signature string `json:"signature"`
	IsAdmin   int8   `json:"is_admin"`
	Status    int8   `json:"status"`
	CreatedAt string `json:"created_at"`
}

type UserListItem struct {
	Uuid      string `json:"uuid"`
	Telephone string `json:"telephone"`
	Nickname  string `json:"nickname"`
	Status    int8   `json:"status"`
	IsAdmin   int8   `json:"is_admin"`
	IsDeleted bool   `json:"is_deleted"`
}

type AuthResponse struct {
	Token    string            `json:"token"`
	UserInfo *UserInfoResponse `json:"user_info"`
}

type SearchUserItem struct {
	Uuid      string `json:"uuid"`
	Nickname  string `json:"nickname"`
	Telephone string `json:"telephone"`
	Avatar    string `json:"avatar"`
	IsFriend  bool   `json:"is_friend"`
}

func issueToken(uuid string) string {
	conf := config.Get()
	ttl := time.Duration(conf.Auth.TokenExpireHours) * time.Hour
	return util.SignToken(uuid, conf.Auth.JwtSecret, ttl)
}

func Login(ctx context.Context, telephone string, password string) (string, *AuthResponse, int) {
	var user model.UserInfo
	err := dao.ActiveQuery(&user).Where("telephone", telephone).First(ctx, &user)
	if err != nil {
		log.Println(err)
		return "user not found, please register", nil, -2
	}
	if !util.VerifyPassword(user.Password, password) {
		return "incorrect password", nil, -2
	}
	if user.Status == constant.UserStatusDisable {
		return "account is disabled", nil, -2
	}
	return "login successful", &AuthResponse{Token: issueToken(user.Uuid), UserInfo: toUserInfoResponse(&user)}, 0
}

func Register(ctx context.Context, telephone, password, nickname string) (string, *AuthResponse, int) {
	var existing model.UserInfo
	err := dao.ActiveQuery(&existing).Where("telephone", telephone).First(ctx, &existing)
	if err == nil {
		return "phone number already registered", nil, -2
	}

	user := model.UserInfo{
		Uuid:      "U" + util.GetNowAndLenRandomString(11),
		Telephone: telephone,
		Password:  util.HashPassword(password),
		Nickname:  nickname,
		Avatar:    "",
		CreatedAt: time.Now(),
		IsAdmin:   0,
		Status:    constant.UserStatusNormal,
	}

	if _, err := dao.Engine.Model(&user).Insert(ctx, &user); err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	// Every account starts with a default contact tag, like the legacy "好友".
	tag := model.ContactTag{
		Uuid:      "T" + util.GetNowAndLenRandomString(11),
		UserId:    user.Uuid,
		Name:      "Friends",
		CreatedAt: time.Now(),
	}
	if _, err := dao.Engine.Model(&tag).Insert(ctx, &tag); err != nil {
		log.Printf("Register: default tag insert failed: %v", err)
	}
	return "registration successful", &AuthResponse{Token: issueToken(user.Uuid), UserInfo: toUserInfoResponse(&user)}, 0
}

// UpdatePassword resets the password by telephone. Like the legacy backend's
// forgot-password flow it is deliberately unauthenticated.
func UpdatePassword(ctx context.Context, telephone, password string) (string, int) {
	var user model.UserInfo
	if err := dao.ActiveQuery(&user).Where("telephone", telephone).First(ctx, &user); err != nil {
		return "user not found", -2
	}
	if _, err := dao.Engine.Model(&model.UserInfo{}).Where("uuid", user.Uuid).
		Update(ctx, bson.M{"password": util.HashPassword(password)}); err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	_ = dao.UserInfoCache.Delete(ctx, user.Uuid)
	return "password updated", 0
}

// SearchUsers finds users by telephone or nickname keyword, flagging the ones
// that are already contacts of the caller.
func SearchUsers(ctx context.Context, ownerId, keyword string) (string, []SearchUserItem, int) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "keyword is required", nil, -2
	}
	pattern := primitive.Regex{Pattern: regexp.QuoteMeta(keyword), Options: "i"}
	var users []model.UserInfo
	err := dao.ActiveQuery(&users).
		Where("uuid", "!=", ownerId).
		Where("status", constant.UserStatusNormal).
		Where(bson.M{"$or": bson.A{bson.M{"telephone": pattern}, bson.M{"nickname": pattern}}}).
		Limit(20).
		Find(ctx, &users)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	if len(users) == 0 {
		return "success", nil, 0
	}

	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Uuid)
	}
	friendSet := make(map[string]bool)
	var contacts []model.UserContact
	if err := dao.ActiveQuery(&contacts).
		Where("user_id", ownerId).
		WhereIn("contact_id", ids).
		WhereIn("status", []int8{constant.ContactNormal, constant.ContactBlack, constant.ContactBeBlack}).
		Find(ctx, &contacts); err == nil {
		for _, c := range contacts {
			friendSet[c.ContactId] = true
		}
	}

	var list []SearchUserItem
	for _, u := range users {
		list = append(list, SearchUserItem{
			Uuid: u.Uuid, Nickname: u.Nickname, Telephone: u.Telephone,
			Avatar: u.Avatar, IsFriend: friendSet[u.Uuid],
		})
	}
	return "success", list, 0
}

func UpdateUserInfo(ctx context.Context, uuid string, fields bson.M) (string, int) {
	if len(fields) == 0 {
		return "user info updated", 0
	}
	_, err := dao.Engine.Model(&model.UserInfo{}).Where("uuid", uuid).Update(ctx, fields)
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	_ = dao.UserInfoCache.Delete(ctx, uuid)

	// Keep the denormalized session fields in sync with the user profile.
	sessionFields := bson.M{}
	if nickname, ok := fields["nickname"]; ok {
		sessionFields["receive_name"] = nickname
	}
	if avatar, ok := fields["avatar"]; ok {
		sessionFields["avatar"] = avatar
	}
	if len(sessionFields) > 0 {
		if _, err := dao.Engine.Model(&model.Session{}).
			Where("receive_id", uuid).WhereNull("deleted_at").
			Update(ctx, sessionFields); err != nil {
			log.Printf("UpdateUserInfo: session sync failed: %v", err)
		}
		invalidateSessionCacheByReceiver(ctx, uuid)
	}
	return "user info updated", 0
}

func GetUserInfo(ctx context.Context, uuid string) (string, *UserInfoResponse, int) {
	if view, err := dao.UserInfoCache.Get(ctx, uuid); err == nil {
		var user model.UserInfo
		if err := json.Unmarshal(view.ByteSlice(), &user); err == nil {
			return "user info retrieved", toUserInfoResponse(&user), 0
		}
	}
	var user model.UserInfo
	err := dao.ActiveQuery(&user).Where("uuid", uuid).First(ctx, &user)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	return "user info retrieved", toUserInfoResponse(&user), 0
}

func GetUserInfoList(ctx context.Context, ownerId string) (string, []UserListItem, int) {
	var users []model.UserInfo
	err := dao.Engine.Model(&users).Where("uuid", "!=", ownerId).Find(ctx, &users)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []UserListItem
	for _, u := range users {
		list = append(list, UserListItem{
			Uuid:      u.Uuid,
			Telephone: u.Telephone,
			Nickname:  u.Nickname,
			Status:    u.Status,
			IsAdmin:   u.IsAdmin,
			IsDeleted: u.DeletedAt != nil,
		})
	}
	return "user list retrieved", list, 0
}

func AbleUsers(ctx context.Context, uuidList []string) (string, int) {
	_, err := dao.Engine.Model(&model.UserInfo{}).WhereIn("uuid", uuidList).Update(ctx, bson.M{"status": constant.UserStatusNormal})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	invalidateUserCaches(ctx, uuidList)
	return "users enabled", 0
}

func DisableUsers(ctx context.Context, uuidList []string) (string, int) {
	_, err := dao.Engine.Model(&model.UserInfo{}).WhereIn("uuid", uuidList).Update(ctx, bson.M{"status": constant.UserStatusDisable})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	invalidateUserCaches(ctx, uuidList)
	now := time.Now()
	for _, uuid := range uuidList {
		invalidateSessionCacheByReceiver(ctx, uuid)
		if _, err := dao.Engine.Model(&model.Session{}).
			Where("send_id", uuid).
			WhereNull("deleted_at").
			Update(ctx, bson.M{"deleted_at": now}); err != nil {
			log.Printf("DisableUsers: session cleanup (send) for %s failed: %v", uuid, err)
		}
		if _, err := dao.Engine.Model(&model.Session{}).
			Where("receive_id", uuid).
			WhereNull("deleted_at").
			Update(ctx, bson.M{"deleted_at": now}); err != nil {
			log.Printf("DisableUsers: session cleanup (receive) for %s failed: %v", uuid, err)
		}
	}
	return "users disabled", 0
}

func DeleteUsers(ctx context.Context, uuidList []string) (string, int) {
	now := time.Now()
	_, err := dao.Engine.Model(&model.UserInfo{}).WhereIn("uuid", uuidList).Update(ctx, bson.M{"deleted_at": now})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	invalidateUserCaches(ctx, uuidList)
	for _, uuid := range uuidList {
		invalidateSessionCacheByReceiver(ctx, uuid)
		cleanups := []struct {
			m     any
			field string
		}{
			{&model.Session{}, "send_id"},
			{&model.Session{}, "receive_id"},
			{&model.UserContact{}, "user_id"},
			{&model.UserContact{}, "contact_id"},
			{&model.ContactApply{}, "user_id"},
			{&model.ContactApply{}, "contact_id"},
		}
		for _, c := range cleanups {
			if _, err := dao.Engine.Model(c.m).
				Where(c.field, uuid).WhereNull("deleted_at").
				Update(ctx, bson.M{"deleted_at": now}); err != nil {
				log.Printf("DeleteUsers: cleanup %T.%s for %s failed: %v", c.m, c.field, uuid, err)
			}
		}
	}
	return "users deleted", 0
}

func SetAdmin(ctx context.Context, uuidList []string, isAdmin int8) (string, int) {
	_, err := dao.Engine.Model(&model.UserInfo{}).WhereIn("uuid", uuidList).Update(ctx, bson.M{"is_admin": isAdmin})
	if err != nil {
		log.Println(err)
		return constant.SystemError, -1
	}
	invalidateUserCaches(ctx, uuidList)
	return "admin status updated", 0
}

func invalidateUserCaches(ctx context.Context, uuidList []string) {
	for _, uuid := range uuidList {
		_ = dao.UserInfoCache.Delete(ctx, uuid)
		_ = dao.SessionListCache.Delete(ctx, uuid)
	}
}

func toUserInfoResponse(user *model.UserInfo) *UserInfoResponse {
	year, month, day := user.CreatedAt.Date()
	return &UserInfoResponse{
		Uuid:      user.Uuid,
		Telephone: user.Telephone,
		Nickname:  user.Nickname,
		Email:     user.Email,
		Avatar:    user.Avatar,
		Gender:    user.Gender,
		Birthday:  user.Birthday,
		Signature: user.Signature,
		IsAdmin:   user.IsAdmin,
		Status:    user.Status,
		CreatedAt: fmt.Sprintf("%d.%d.%d", year, month, day),
	}
}
