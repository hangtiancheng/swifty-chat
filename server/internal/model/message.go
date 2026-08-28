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

type Message struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Uuid       string             `bson:"uuid" json:"uuid"`
	SessionId  string             `bson:"session_id" json:"session_id"`
	Type       int8               `bson:"type" json:"type"`
	Content    string             `bson:"content" json:"content"`
	Url        string             `bson:"url" json:"url"`
	SendId     string             `bson:"send_id" json:"send_id"`
	SendName   string             `bson:"send_name" json:"send_name"`
	SendAvatar string             `bson:"send_avatar" json:"send_avatar"`
	ReceiveId  string             `bson:"receive_id" json:"receive_id"`
	FileType   string             `bson:"file_type" json:"file_type"`
	FileName   string             `bson:"file_name" json:"file_name"`
	FileSize   string             `bson:"file_size" json:"file_size"`
	Status     int8               `bson:"status" json:"status"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
	SendAt     *time.Time         `bson:"send_at,omitempty" json:"send_at"`
	AVdata     string             `bson:"av_data" json:"av_data"`
}
