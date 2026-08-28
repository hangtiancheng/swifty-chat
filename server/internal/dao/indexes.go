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
	"log"

	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func uniqueUuidIndex() mongo.IndexModel {
	return mongo.IndexModel{
		Keys:    bson.D{{Key: "uuid", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
}

func compoundIndex(fields ...string) mongo.IndexModel {
	keys := bson.D{}
	for _, f := range fields {
		keys = append(keys, bson.E{Key: f, Value: 1})
	}
	return mongo.IndexModel{Keys: keys}
}

// InitIndexes creates the indexes the query paths rely on. The unique uuid
// index also guards against random-id collisions.
func InitIndexes() {
	ctx := context.Background()
	specs := []struct {
		model   any
		indexes []mongo.IndexModel
	}{
		{&model.UserInfo{}, []mongo.IndexModel{
			uniqueUuidIndex(),
			compoundIndex("telephone"),
		}},
		{&model.GroupInfo{}, []mongo.IndexModel{
			uniqueUuidIndex(),
			compoundIndex("owner_id"),
		}},
		{&model.Session{}, []mongo.IndexModel{
			uniqueUuidIndex(),
			compoundIndex("send_id", "receive_id"),
			compoundIndex("receive_id"),
		}},
		{&model.Message{}, []mongo.IndexModel{
			uniqueUuidIndex(),
			compoundIndex("send_id", "receive_id", "created_at"),
			compoundIndex("receive_id", "created_at"),
		}},
		{&model.UserContact{}, []mongo.IndexModel{
			compoundIndex("user_id", "contact_id"),
			compoundIndex("contact_id"),
		}},
		{&model.ContactApply{}, []mongo.IndexModel{
			uniqueUuidIndex(),
			compoundIndex("contact_id", "status"),
			compoundIndex("user_id", "contact_id"),
		}},
		{&model.ContactTag{}, []mongo.IndexModel{
			uniqueUuidIndex(),
			compoundIndex("user_id"),
		}},
	}
	for _, spec := range specs {
		if _, err := Engine.Model(spec.model).EnsureIndexes(ctx, spec.indexes); err != nil {
			log.Printf("ensure indexes for %T failed: %v", spec.model, err)
		}
	}
}
