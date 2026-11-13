package apisdk

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// A client is identified by a private user token that only it knows (send it
// with option.WithUserToken). The public user ID that rooms, messages and
// events show is derived from the token by a one-way hash, so sharing your
// ID never lets anyone act as you. Keep the token secret.

// UserTokenHeader is the header the API reads the user token from.
const UserTokenHeader = "X-User-Token" //nolint:gosec // G101: a header name, not a credential

// MinUserTokenLength is the shortest token the API accepts.
const MinUserTokenLength = 16

// NewUserToken returns a fresh random user token (128 bits).
func NewUserToken() string {
	return rand.Text()
}

// UserIDFromToken returns the public user ID the API derives from token: the
// first 16 bytes of SHA-256("visper-user-id:" || token) as a version 8 UUID.
func UserIDFromToken(token string) string {
	sum := sha256.Sum256([]byte("visper-user-id:" + token))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x80
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
