package room

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

func generateSecureCode() string {
	const tokenLength = 32

	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(time.Now().UnixNano() % 256)
		}
	}

	return base64.RawURLEncoding.EncodeToString(b)
}

func generateJoinCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Exclude similar looking chars
	const codeLength = 6

	code := make([]byte, codeLength)
	for i := range code {
		code[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond) // Ensure different values for the time
	}

	return string(code)
}
