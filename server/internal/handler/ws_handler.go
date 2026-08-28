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
	"github.com/hangtiancheng/swifty-chat/server/internal/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/service"
	"github.com/hangtiancheng/swifty-chat/server/internal/util"

	"github.com/hangtiancheng/swifty.go/swifty_http"
)

// WsLogin authenticates the handshake before upgrading. The identity always
// comes from the token: trusting a client-supplied id would let anyone who
// knows a uuid evict that user's socket in register() and receive their
// messages. Browsers cannot set headers on a websocket handshake, so the
// token rides in the query string.
func WsLogin(ctx *swifty_http.Context, next func()) {
	claims, err := util.ParseToken(ctx.Query("token"), config.Get().Auth.JwtSecret)
	if err != nil {
		JsonStatus(ctx, 401, "invalid or expired token")
		return
	}
	if clientId := ctx.Query("client_id"); clientId != "" && clientId != claims.Uuid {
		JsonStatus(ctx, 403, "client_id does not match the token")
		return
	}
	ws, err := ctx.Upgrade(&swifty_http.UpgradeOptions{
		ReadBufferSize:  2048,
		WriteBufferSize: 2048,
	})
	if err != nil {
		return
	}
	service.NewClientInit(ws, claims.Uuid)
}

// WsLogout disconnects the caller's own socket. owner_id is accepted for
// backwards compatibility but must match the token, so one user cannot force
// another offline.
func WsLogout(ctx *swifty_http.Context, next func()) {
	var req struct {
		OwnerId string `json:"owner_id"`
	}
	if err := ctx.BindJSON(&req); err != nil {
		JsonBack(ctx, "invalid request body", -1, nil)
		return
	}
	uuid, _ := ctx.State["uuid"].(string)
	if req.OwnerId != "" && req.OwnerId != uuid {
		JsonStatus(ctx, 403, "owner_id does not match the token")
		return
	}
	msg, ret := service.ClientLogout(uuid)
	JsonBack(ctx, msg, ret, nil)
}
