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
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/constant"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"github.com/hangtiancheng/swifty.go/swifty_orm"
)

// EnsureSwiftxUser creates the reserved Swiftx assistant account on startup.
func EnsureSwiftxUser(ctx context.Context) {
	var existing model.UserInfo
	err := dao.Engine.Model(&existing).Where("uuid", constant.SwiftxUUID).First(ctx, &existing)
	if err == nil {
		return
	}
	if err != swifty_orm.ErrNotFound {
		log.Printf("EnsureSwiftxUser: lookup failed: %v", err)
		return
	}
	user := model.UserInfo{
		Uuid:      constant.SwiftxUUID,
		Nickname:  constant.SwiftxName,
		Signature: constant.SwiftxSignature,
		CreatedAt: time.Now(),
		Status:    constant.UserStatusNormal,
	}
	if _, err := dao.Engine.Model(&user).Insert(ctx, &user); err != nil {
		log.Printf("EnsureSwiftxUser: insert failed: %v", err)
		return
	}
	log.Printf("Swiftx assistant account created (%s)", constant.SwiftxUUID)
}

// EnsureSwiftxContact idempotently gives a user the undeletable Swiftx
// contact and a session pointing at it. Called on register and login so
// existing accounts pick it up too.
func EnsureSwiftxContact(ctx context.Context, userId string) {
	if userId == "" || userId == constant.SwiftxUUID {
		return
	}
	if err := ensureUserContact(ctx, dao.Engine, userId, constant.SwiftxUUID); err != nil {
		log.Printf("EnsureSwiftxContact %s: contact failed: %v", userId, err)
		return
	}
	if err := ensureUserContact(ctx, dao.Engine, constant.SwiftxUUID, userId); err != nil {
		log.Printf("EnsureSwiftxContact %s: reverse contact failed: %v", userId, err)
	}
	ensurePeerSession(ctx, userId, constant.SwiftxUUID)
}

// IsSwiftx reports whether the given uuid is the built-in assistant.
func IsSwiftx(uuid string) bool {
	return uuid == constant.SwiftxUUID
}
