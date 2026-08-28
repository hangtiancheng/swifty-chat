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

package dao

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/config"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"github.com/hangtiancheng/swifty.go/swifty_cache"
)

var (
	// UserInfoCache caches user documents by uuid (read-through).
	UserInfoCache *swifty_cache.Group
	// SessionListCache caches each user's active session list by owner id
	// (read-through). Both the user and group session endpoints share it.
	SessionListCache *swifty_cache.Group
)

func InitCache() {
	conf := config.Get()
	maxBytes := conf.Cache.MaxBytes
	expiration := time.Duration(conf.Cache.Expiration) * time.Second
	opts := []swifty_cache.GroupOption{swifty_cache.WithExpiration(expiration)}

	UserInfoCache = swifty_cache.NewGroup("user_info", maxBytes, swifty_cache.GetterFunc(
		func(ctx context.Context, key string) ([]byte, error) {
			if key == "" {
				return nil, swifty_cache.ErrKeyRequired
			}
			var user model.UserInfo
			if err := ActiveQuery(&user).Where("uuid", key).First(ctx, &user); err != nil {
				return nil, err
			}
			return json.Marshal(&user)
		},
	), opts...)

	SessionListCache = swifty_cache.NewGroup("session_list", maxBytes, swifty_cache.GetterFunc(
		func(ctx context.Context, key string) ([]byte, error) {
			if key == "" {
				return nil, swifty_cache.ErrKeyRequired
			}
			var sessions []model.Session
			if err := ActiveQuery(&sessions).
				Where("send_id", key).
				OrderBy("created_at", "desc").
				Find(ctx, &sessions); err != nil {
				return nil, err
			}
			return json.Marshal(sessions)
		},
	), opts...)

	log.Printf("cache initialized: maxBytes=%d, expiration=%v", maxBytes, expiration)
}

func CloseCache() {
	swifty_cache.DestroyAllGroups()
}
