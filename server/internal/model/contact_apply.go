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

type ContactApply struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Uuid        string             `bson:"uuid" json:"uuid"`
	UserId      string             `bson:"user_id" json:"user_id"`
	ContactId   string             `bson:"contact_id" json:"contact_id"`
	ContactType int8               `bson:"contact_type" json:"contact_type"`
	Status      int8               `bson:"status" json:"status"`
	Message     string             `bson:"message" json:"message"`
	LastApplyAt time.Time          `bson:"last_apply_at" json:"last_apply_at"`
	DeletedAt   *time.Time         `bson:"deleted_at,omitempty" json:"-"`
}
