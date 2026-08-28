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

	"github.com/hangtiancheng/swifty-chat/server/internal/constant"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"go.mongodb.org/mongo-driver/bson"
)

type MessageListItem struct {
	Uuid       string `json:"uuid"`
	SendId     string `json:"send_id"`
	SendName   string `json:"send_name"`
	SendAvatar string `json:"send_avatar"`
	ReceiveId  string `json:"receive_id"`
	Type       int8   `json:"type"`
	Content    string `json:"content"`
	Url        string `json:"url"`
	FileSize   string `json:"file_size"`
	FileName   string `json:"file_name"`
	FileType   string `json:"file_type"`
	CreatedAt  string `json:"created_at"`
	AVdata     string `json:"av_data,omitempty"`
}

func toMessageListItem(m *model.Message) MessageListItem {
	return MessageListItem{
		Uuid:   m.Uuid,
		SendId: m.SendId, SendName: m.SendName, SendAvatar: m.SendAvatar,
		ReceiveId: m.ReceiveId, Type: m.Type, Content: m.Content,
		Url: m.Url, FileSize: m.FileSize, FileName: m.FileName,
		FileType: m.FileType, CreatedAt: m.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func GetMessageList(ctx context.Context, sendId, receiveId string) (string, []MessageListItem, int) {
	var messages []model.Message
	err := dao.Engine.Model(&messages).
		Where("send_id", sendId).
		Where("receive_id", receiveId).
		OrWhere(bson.M{"send_id": receiveId, "receive_id": sendId}).
		OrderBy("created_at", "asc").
		Find(ctx, &messages)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}

	var list []MessageListItem
	for i := range messages {
		list = append(list, toMessageListItem(&messages[i]))
	}
	return "success", list, 0
}

func GetGroupMessageList(ctx context.Context, groupId string) (string, []MessageListItem, int) {
	var messages []model.Message
	err := dao.Engine.Model(&messages).
		Where("receive_id", groupId).
		OrderBy("created_at", "asc").
		Find(ctx, &messages)
	if err != nil {
		log.Println(err)
		return constant.SystemError, nil, -1
	}
	var list []MessageListItem
	for i := range messages {
		list = append(list, toMessageListItem(&messages[i]))
	}
	return "success", list, 0
}
