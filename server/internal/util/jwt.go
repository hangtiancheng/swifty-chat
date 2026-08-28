package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

type TokenClaims struct {
	Uuid string `json:"uuid"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// SignToken issues a HS256 JWT for the given user uuid.
func SignToken(uuid, secret string, ttl time.Duration) string {
	now := time.Now()
	claims := TokenClaims{Uuid: uuid, Iat: now.Unix(), Exp: now.Add(ttl).Unix()}
	payload, _ := json.Marshal(claims)
	signing := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	return signing + "." + sign(signing, secret)
}

// ParseToken validates the signature and expiry and returns the claims.
func ParseToken(token, secret string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	signing := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(sign(signing, secret)), []byte(parts[2])) {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.Uuid == "" {
		return nil, ErrInvalidToken
	}
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, ErrExpiredToken
	}
	return &claims, nil
}

func sign(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
