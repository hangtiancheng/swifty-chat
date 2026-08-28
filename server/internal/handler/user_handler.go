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

func Login(ctx *swifty_http.Context, next func()) {
	var req struct {
		Telephone string `json:"telephone"`
		Password  string `json:"password"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.Login(ctx.Request.Context(), req.Telephone, req.Password)
	JsonBack(ctx, msg, ret, data)
}

func Register(ctx *swifty_http.Context, next func()) {
	var req struct {
		Telephone string `json:"telephone"`
		Password  string `json:"password"`
		Nickname  string `json:"nickname"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.Register(ctx.Request.Context(), req.Telephone, req.Password, req.Nickname)
	JsonBack(ctx, msg, ret, data)
}

func UpdatePassword(ctx *swifty_http.Context, next func()) {
	var req struct {
		Telephone string `json:"telephone"`
		Password  string `json:"password"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	if req.Telephone == "" || req.Password == "" {
		JsonBack(ctx, "telephone and password are required", -2, nil)
		return
	}
	msg, ret := service.UpdatePassword(ctx.Request.Context(), req.Telephone, req.Password)
	JsonBack(ctx, msg, ret, nil)
}

func SearchUser(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
		Keyword string `json:"keyword"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.SearchUsers(ctx.Request.Context(), req.OwnerId, req.Keyword)
	JsonBack(ctx, msg, ret, data)
}

func UpdateUserInfo(ctx *swifty_http.Context, next func()) {
	var req struct {
		Uuid      string `json:"uuid"`
		Nickname  string `json:"nickname"`
		Email     string `json:"email"`
		Birthday  string `json:"birthday"`
		Signature string `json:"signature"`
		Avatar    string `json:"avatar"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	fields := bson.M{}
	if req.Nickname != "" {
		fields["nickname"] = req.Nickname
	}
	if req.Email != "" {
		fields["email"] = req.Email
	}
	if req.Birthday != "" {
		fields["birthday"] = req.Birthday
	}
	if req.Signature != "" {
		fields["signature"] = req.Signature
	}
	if req.Avatar != "" {
		fields["avatar"] = req.Avatar
	}
	msg, ret := service.UpdateUserInfo(ctx.Request.Context(), req.Uuid, fields)
	JsonBack(ctx, msg, ret, nil)
}

func GetUserInfo(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetUserInfo(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func GetUserInfoList(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetUserInfoList(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func AbleUsers(ctx *swifty_http.Context, next func()) {
	var req struct {
		UuidList []string `json:"uuid_list"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.AbleUsers(ctx.Request.Context(), req.UuidList)
	JsonBack(ctx, msg, ret, nil)
}

func DisableUsers(ctx *swifty_http.Context, next func()) {
	var req struct {
		UuidList []string `json:"uuid_list"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.DisableUsers(ctx.Request.Context(), req.UuidList)
	JsonBack(ctx, msg, ret, nil)
}

func DeleteUsers(ctx *swifty_http.Context, next func()) {
	var req struct {
		UuidList []string `json:"uuid_list"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.DeleteUsers(ctx.Request.Context(), req.UuidList)
	JsonBack(ctx, msg, ret, nil)
}

func SetAdmin(ctx *swifty_http.Context, next func()) {
	var req struct {
		UuidList []string `json:"uuid_list"`
		IsAdmin  int8     `json:"is_admin"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.SetAdmin(ctx.Request.Context(), req.UuidList, req.IsAdmin)
	JsonBack(ctx, msg, ret, nil)
}
