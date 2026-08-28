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

// AgentWs carries Swiftx's progress for one client: streaming text, thinking,
// tool calls, and the permission and question prompts a run blocks on. Prompts
// themselves travel the ordinary chat socket, so nothing here can put words in
// the transcript.
//
// The identity comes from the token for the same reason /wss does: it decides
// whose agent, whose workspace and whose approvals this socket speaks for.
// Browsers cannot set handshake headers, so the token rides in the query string.
func AgentWs(ctx *swifty_http.Context, next func()) {
	claims, err := util.ParseToken(ctx.Query("token"), config.Get().Auth.JwtSecret)
	if err != nil {
		JsonStatus(ctx, 401, "invalid or expired token")
		return
	}
	if service.AgentHub == nil {
		JsonStatus(ctx, 503, "the assistant is not configured on this server")
		return
	}
	ws, err := ctx.Upgrade(&swifty_http.UpgradeOptions{
		MaxMessageSize: 4 << 20,
	})
	if err != nil {
		return
	}
	service.AgentHub.Serve(claims.Uuid, ws)
}
