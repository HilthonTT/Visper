package room

import (
	"crypto/rand"
	"encoding/base64"
)

const (
	// joinCodeCharset excludes look-alike characters. Its length (32) divides
	// 256, so mapping a random byte with % is unbiased.
	joinCodeCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	joinCodeLength  = 6

	secureCodeLength = 32
)

// generateSecureCode returns a URL-safe random token. crypto/rand.Read never
// returns an error (it aborts the program if the OS RNG is unavailable).
func generateSecureCode() string {
	b := make([]byte, secureCodeLength)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// generateJoinCode returns a random, human friendly join code. It uses
// crypto/rand: join codes grant access to rooms and must not be predictable.
func generateJoinCode() string {
	b := make([]byte, joinCodeLength)
	_, _ = rand.Read(b)

	for i := range b {
		b[i] = joinCodeCharset[int(b[i])%len(joinCodeCharset)]
	}

	return string(b)
}
