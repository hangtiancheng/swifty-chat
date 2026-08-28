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
	"github.com/hangtiancheng/swifty-chat/server/internal/service"

	"github.com/hangtiancheng/swifty.go/swifty_http"
)

func GetUserList(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetUserList(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func GetTagList(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetTagList(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func AddTag(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
		Name    string `json:"name"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.AddTag(ctx.Request.Context(), req.OwnerId, req.Name)
	JsonBack(ctx, msg, ret, data)
}

func UpdateContact(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId    string  `json:"user_id"`
		ContactId string  `json:"contact_id"`
		NoteName  *string `json:"note_name"`
		TagId     *string `json:"tag_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.UpdateContact(ctx.Request.Context(), req.UserId, req.ContactId, req.NoteName, req.TagId)
	JsonBack(ctx, msg, ret, nil)
}

func LoadMyJoinedGroup(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.LoadMyJoinedGroup(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func GetContactInfo(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId    string `json:"user_id"`
		ContactId string `json:"contact_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetContactInfo(ctx.Request.Context(), req.UserId, req.ContactId)
	JsonBack(ctx, msg, ret, data)
}

func ApplyContact(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId      string `json:"user_id"`
		ContactId   string `json:"contact_id"`
		ContactType int8   `json:"contact_type"`
		Message     string `json:"message"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.ApplyContact(ctx.Request.Context(), req.UserId, req.ContactId, req.ContactType, req.Message)
	JsonBack(ctx, msg, ret, nil)
}

func GetNewContactList(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId string `json:"user_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetNewContactList(ctx.Request.Context(), req.UserId)
	JsonBack(ctx, msg, ret, data)
}

func PassContactApply(ctx *swifty_http.Context, next func()) {
	var req struct {
		ApplyId string `json:"apply_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.PassContactApply(ctx.Request.Context(), req.ApplyId)
	JsonBack(ctx, msg, ret, nil)
}

func BlackContact(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId    string `json:"user_id"`
		ContactId string `json:"contact_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.BlackContact(ctx.Request.Context(), req.UserId, req.ContactId)
	JsonBack(ctx, msg, ret, nil)
}

func CancelBlackContact(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId    string `json:"user_id"`
		ContactId string `json:"contact_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.CancelBlackContact(ctx.Request.Context(), req.UserId, req.ContactId)
	JsonBack(ctx, msg, ret, nil)
}

func DeleteContact(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId    string `json:"user_id"`
		ContactId string `json:"contact_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.DeleteContact(ctx.Request.Context(), req.UserId, req.ContactId)
	JsonBack(ctx, msg, ret, nil)
}

func RefuseContactApply(ctx *swifty_http.Context, next func()) {
	var req struct {
		ApplyId string `json:"apply_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.RefuseContactApply(ctx.Request.Context(), req.ApplyId)
	JsonBack(ctx, msg, ret, nil)
}

func BlackApply(ctx *swifty_http.Context, next func()) {
	var req struct {
		ApplyId string `json:"apply_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.BlackApply(ctx.Request.Context(), req.ApplyId)
	JsonBack(ctx, msg, ret, nil)
}

func GetAddGroupList(ctx *swifty_http.Context, next func()) {
	var req struct {
		UserId string `json:"user_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetAddGroupList(ctx.Request.Context(), req.UserId)
	JsonBack(ctx, msg, ret, data)
}
