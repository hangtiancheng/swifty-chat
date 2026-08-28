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

func OpenSession(ctx *swifty_http.Context, next func()) {
	var req struct {
		SendId    string `json:"send_id"`
		ReceiveId string `json:"receive_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, sessionId, ret := service.OpenSession(ctx.Request.Context(), req.SendId, req.ReceiveId)
	JsonBack(ctx, msg, ret, sessionId)
}

func GetUserSessionList(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetUserSessionList(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func GetGroupSessionList(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, data, ret := service.GetGroupSessionList(ctx.Request.Context(), req.OwnerId)
	JsonBack(ctx, msg, ret, data)
}

func DeleteSession(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId   string `json:"owner_id"`
		SessionId string `json:"session_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.DeleteSession(ctx.Request.Context(), req.OwnerId, req.SessionId)
	JsonBack(ctx, msg, ret, nil)
}

func CheckOpenSessionAllowed(ctx *swifty_http.Context, next func()) {
	var req struct {
		SendId    string `json:"send_id"`
		ReceiveId string `json:"receive_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, allowed, ret := service.CheckOpenSessionAllowed(ctx.Request.Context(), req.SendId, req.ReceiveId)
	JsonBack(ctx, msg, ret, allowed)
}

func MarkSessionRead(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId   string `json:"owner_id"`
		ReceiveId string `json:"receive_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	msg, ret := service.MarkSessionRead(ctx.Request.Context(), req.OwnerId, req.ReceiveId)
	JsonBack(ctx, msg, ret, nil)
}
