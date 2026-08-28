package util

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashPassword returns "salt$digest" where digest = sha256(salt + password).
func HashPassword(password string) string {
	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := hex.EncodeToString(saltBytes)
	return salt + "$" + hashWithSalt(salt, password)
}

func VerifyPassword(stored, password string) bool {
	salt, digest, ok := strings.Cut(stored, "$")
	if !ok {
		return false
	}
	return hmac.Equal([]byte(digest), []byte(hashWithSalt(salt, password)))
}

func hashWithSalt(salt, password string) string {
	sum := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(sum[:])
}
