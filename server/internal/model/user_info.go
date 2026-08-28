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

package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UserInfo struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Uuid          string             `bson:"uuid" json:"uuid"`
	Nickname      string             `bson:"nickname" json:"nickname"`
	Telephone     string             `bson:"telephone" json:"telephone"`
	Email         string             `bson:"email" json:"email"`
	Avatar        string             `bson:"avatar" json:"avatar"`
	Gender        int8               `bson:"gender" json:"gender"`
	Signature     string             `bson:"signature" json:"signature"`
	Password      string             `bson:"password" json:"-"`
	Birthday      string             `bson:"birthday" json:"birthday"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	DeletedAt     *time.Time         `bson:"deleted_at,omitempty" json:"-"`
	LastOnlineAt  *time.Time         `bson:"last_online_at,omitempty" json:"last_online_at"`
	LastOfflineAt *time.Time         `bson:"last_offline_at,omitempty" json:"last_offline_at"`
	IsAdmin       int8               `bson:"is_admin" json:"is_admin"`
	Status        int8               `bson:"status" json:"status"`
}
