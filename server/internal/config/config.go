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

package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
)

type AppConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type MongoConfig struct {
	URI      string `json:"uri"`
	Database string `json:"database"`
}

type CacheConfig struct {
	MaxBytes   int64 `json:"maxBytes"`
	Expiration int   `json:"expiration"`
}

type StaticConfig struct {
	AvatarPath string `json:"avatarPath"`
	FilePath   string `json:"filePath"`
	ChunkPath  string `json:"chunkPath"`
}

type AuthConfig struct {
	JwtSecret        string `json:"jwtSecret"`
	TokenExpireHours int    `json:"tokenExpireHours"`
}

type Config struct {
	App    AppConfig    `json:"app"`
	Mongo  MongoConfig  `json:"mongo"`
	Cache  CacheConfig  `json:"cache"`
	Static StaticConfig `json:"static"`
	Auth   AuthConfig   `json:"auth"`
}

var conf *Config

func Load(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("failed to read config: %v", err)
	}
	conf = &Config{}
	if err := json.Unmarshal(data, conf); err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}
	if conf.Auth.JwtSecret == "" {
		// Tokens signed with an ephemeral secret become invalid on restart.
		buf := make([]byte, 32)
		_, _ = rand.Read(buf)
		conf.Auth.JwtSecret = hex.EncodeToString(buf)
		log.Println("auth.jwtSecret not configured; using an ephemeral secret (tokens expire on restart)")
	}
	if conf.Auth.TokenExpireHours <= 0 {
		conf.Auth.TokenExpireHours = 14 * 24
	}
	if conf.Static.ChunkPath == "" {
		conf.Static.ChunkPath = "./static/chunks"
	}
	return conf
}

func Get() *Config {
	if conf == nil {
		log.Fatal("config not loaded")
	}
	return conf
}
