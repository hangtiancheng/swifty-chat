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
	"go.mongodb.org/mongo-driver/bson"

	"github.com/hangtiancheng/swifty-chat/server/internal/service"

	"github.com/hangtiancheng/swifty.go/swifty_http"
)

func CreateGroup(ctx *swifty_http.Context, next func()) {
	var req struct {
		Name      string   `json:"name"`
		OwnerId   string   `json:"owner_id"`
		Avatar    string   `json:"avatar"`
		Notice    string   `json:"notice"`
		AddMode   int8     `json:"add_mode"`
		MemberIds []string `json:"member_ids"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.CreateGroup(ctx.Request.Context(), req.Name, req.OwnerId, req.Avatar, req.Notice, req.AddMode, req.MemberIds)
	JsonBack(ctx, msg, ret, data)
}

func InviteGroupMembers(ctx *swifty_http.Context, next func()) {
	var req struct {
		GroupId   string   `json:"group_id"`
		MemberIds []string `json:"member_ids"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	if len(req.MemberIds) == 0 {
		JsonBack(ctx, "member_ids is required", -2, nil)
		return
	}
	msg, ret := service.InviteGroupMembers(ctx.Request.Context(), req.GroupId, req.MemberIds)
	JsonBack(ctx, msg, ret, nil)
}

func SearchGroup(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
		Keyword string `json:"keyword"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.SearchGroups(ctx.Request.Context(), req.OwnerId, req.Keyword)
	JsonBack(ctx, msg, ret, data)
}

func LoadMyGroup(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.LoadMyGroup(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func GetGroupInfo(ctx *swifty_http.Context, next func()) {
	var req struct {
		GroupId string `json:"group_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetGroupInfo(ctx.Request.Context(), req.GroupId)
	JsonBack(ctx, msg, ret, data)
}

func CheckGroupAddMode(ctx *swifty_http.Context, next func()) {
	var req struct {
		GroupId string `json:"group_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, addMode, ret := service.CheckGroupAddMode(ctx.Request.Context(), req.GroupId)
	JsonBack(ctx, msg, ret, addMode)
}

func EnterGroupDirectly(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId  string `json:"user_id"`
		GroupId string `json:"group_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.EnterGroupDirectly(ctx.Request.Context(), req.UserId, req.GroupId)
	JsonBack(ctx, msg, ret, nil)
}

func LeaveGroup(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId  string `json:"user_id"`
		GroupId string `json:"group_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.LeaveGroup(ctx.Request.Context(), req.UserId, req.GroupId)
	JsonBack(ctx, msg, ret, nil)
}

func DismissGroup(ctx *swifty_http.Context, next func()) {
	var req struct {
		GroupId string `json:"group_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.DismissGroup(ctx.Request.Context(), req.GroupId)
	JsonBack(ctx, msg, ret, nil)
}

func UpdateGroupInfo(ctx *swifty_http.Context, next func()) {
	var req struct {
		Uuid    string `json:"uuid"`
		Name    string `json:"name"`
		Notice  string `json:"notice"`
		Avatar  string `json:"avatar"`
		AddMode *int8  `json:"add_mode"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	fields := bson.M{}
	if req.Name != "" {
		fields["name"] = req.Name
	}
	if req.Notice != "" {
		fields["notice"] = req.Notice
	}
	if req.Avatar != "" {
		fields["avatar"] = req.Avatar
	}
	if req.AddMode != nil {
		if *req.AddMode != 0 && *req.AddMode != 1 {
			JsonBack(ctx, "invalid add_mode", -2, nil)
			return
		}
		fields["add_mode"] = *req.AddMode
	}
	msg, ret := service.UpdateGroupInfo(ctx.Request.Context(), req.Uuid, fields)
	JsonBack(ctx, msg, ret, nil)
}

func GetGroupMemberList(ctx *swifty_http.Context, next func()) {
	var req struct {
		GroupId string `json:"group_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetGroupMemberList(ctx.Request.Context(), req.GroupId)
	JsonBack(ctx, msg, ret, data)
}

func RemoveGroupMembers(ctx *swifty_http.Context, next func()) {
	var req struct {
		GroupId   string   `json:"group_id"`
		MemberIds []string `json:"member_ids"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.RemoveGroupMembers(ctx.Request.Context(), req.GroupId, req.MemberIds)
	JsonBack(ctx, msg, ret, nil)
}

func GetGroupInfoList(ctx *swifty_http.Context, next func()) {
	msg, data, ret := service.GetGroupInfoList(ctx.Request.Context())
	JsonBack(ctx, msg, ret, data)
}

func DeleteGroups(ctx *swifty_http.Context, next func()) {
	var req struct {
		UuidList []string `json:"uuid_list"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.DeleteGroups(ctx.Request.Context(), req.UuidList)
	JsonBack(ctx, msg, ret, nil)
}

func SetGroupsStatus(ctx *swifty_http.Context, next func()) {
	var req struct {
		UuidList []string `json:"uuid_list"`
		Status   int8     `json:"status"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.SetGroupsStatus(ctx.Request.Context(), req.UuidList, req.Status)
	JsonBack(ctx, msg, ret, nil)
}
