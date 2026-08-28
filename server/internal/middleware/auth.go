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

package middleware

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hangtiancheng/swifty-chat/server/internal/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"
	"github.com/hangtiancheng/swifty-chat/server/internal/util"

	"github.com/hangtiancheng/swifty.go/swifty_http"
)

// publicPaths lists the POST endpoints reachable without a token.
var publicPaths = map[string]bool{
	"/login":                true,
	"/register":             true,
	"/user/update-password": true,
}

// Auth validates the Authorization header on every POST endpoint except the
// public ones. GET endpoints (websocket upgrades, static files, dashboard)
// stay tokenless, matching the legacy behavior.
func Auth() swifty_http.Middleware {
	return func(ctx *swifty_http.Context, next func()) {
		if ctx.Method != "POST" || publicPaths[ctx.Path] {
			next()
			return
		}
		token := strings.TrimPrefix(ctx.Get("Authorization"), "Bearer ")
		if token == "" {
			unauthorized(ctx, "missing token")
			return
		}
		claims, err := util.ParseToken(token, config.Get().Auth.JwtSecret)
		if err != nil {
			unauthorized(ctx, "invalid or expired token")
			return
		}
		ctx.State["uuid"] = claims.Uuid
		next()
	}
}

// RequireAdmin wraps a handler so only admins can invoke it. The admin flag
// is read fresh from the user cache, so revoking admin takes effect without
// re-issuing tokens.
func RequireAdmin(h swifty_http.Middleware) swifty_http.Middleware {
	return func(ctx *swifty_http.Context, next func()) {
		uuid, _ := ctx.State["uuid"].(string)
		if uuid == "" || !isAdmin(ctx.Request.Context(), uuid) {
			ctx.Status = 200
			ctx.JSON(swifty_http.H{"code": 403, "message": "admin privilege required"})
			return
		}
		h(ctx, next)
	}
}

func isAdmin(ctx context.Context, uuid string) bool {
	if view, err := dao.UserInfoCache.Get(ctx, uuid); err == nil {
		var user model.UserInfo
		if err := json.Unmarshal(view.ByteSlice(), &user); err == nil {
			return user.IsAdmin == 1
		}
	}
	var user model.UserInfo
	if err := dao.ActiveQuery(&user).Where("uuid", uuid).First(ctx, &user); err != nil {
		return false
	}
	return user.IsAdmin == 1
}

func unauthorized(ctx *swifty_http.Context, msg string) {
	ctx.Status = 200
	ctx.JSON(swifty_http.H{"code": 401, "message": msg})
}
