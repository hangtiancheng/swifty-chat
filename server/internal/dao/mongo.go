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

	"github.com/hangtiancheng/swifty-chat/server/internal/config"

	"github.com/hangtiancheng/swifty.go/swifty_orm"
)

var Engine *swifty_orm.Engine

func InitMongo() {
	conf := config.Get()
	var err error
	Engine, err = swifty_orm.NewEngine(context.Background(), conf.Mongo.URI, conf.Mongo.Database)
	if err != nil {
		log.Fatalf("failed to connect mongo: %v", err)
	}
	log.Printf("connected to mongodb: %s/%s", conf.Mongo.URI, conf.Mongo.Database)
}

func CloseMongo() {
	if Engine != nil {
		_ = Engine.Close(context.Background())
	}
}
