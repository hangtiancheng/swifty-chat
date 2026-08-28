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

func GetOnlineUsers(ctx *swifty_http.Context, next func()) {
	users := service.GetOnlineUserList()
	JsonBack(ctx, "success", 0, users)
}

// GetCallers lists the other participants of a call room. The room id for a
// group call is just the group uuid, so membership is checked to keep this
// from becoming a presence probe for arbitrary rooms.
func GetCallers(ctx *swifty_http.Context, next func()) {
	var req struct {
		RoomId string `json:"room_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	if req.RoomId == "" {
		JsonBack(ctx, "room_id is required", -2, nil)
		return
	}
	ownerId, _ := ctx.State["uuid"].(string)
	if !service.CanSeeCallRoom(ctx.Request.Context(), req.RoomId, ownerId) {
		JsonStatus(ctx, 403, "not a participant of this call room")
		return
	}
	JsonBack(ctx, "success", 0, service.GetCallers(req.RoomId, ownerId))
}
