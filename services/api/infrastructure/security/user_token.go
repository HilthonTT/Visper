package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
)

// A user is identified by a private token that only its client knows. The
// public user ID (shown to rooms, in messages, presence and so on) is derived
// from the token by a one-way hash, so knowing someone's ID does not let you
// act as them.
const (
	// UserTokenHeader carries the token for API and WebSocket clients.
	UserTokenHeader = "X-User-Token" //nolint:gosec // G101: a header name, not a credential
	// LegacyUserTokenHeader is the header older clients send the token in.
	LegacyUserTokenHeader = "X-User-ID"

	// MinUserTokenLength rejects tokens short enough to guess.
	MinUserTokenLength = 16
	// MaxUserTokenLength bounds what is hashed and stored in the cookie.
	MaxUserTokenLength = 256

	userIDDomain = "visper-user-id:"
)

// NewUserToken returns a fresh random token (128 bits).
func NewUserToken() string {
	return rand.Text()
}

// ValidUserToken reports whether token is acceptable as a credential.
func ValidUserToken(token string) bool {
	return len(token) >= MinUserTokenLength && len(token) <= MaxUserTokenLength
}

// UserIDFromToken derives the public user ID for token: the first 16 bytes
// of SHA-256("visper-user-id:" || token), formatted as a version 8 UUID.
// The Go SDK and the AI service derive it the same way.
func UserIDFromToken(token string) string {
	sum := sha256.Sum256([]byte(userIDDomain + token))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x80 // version 8 (custom)
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 9562 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// GetUserToken returns the token sent with r: the header, or else the
// session cookie. It returns "" if there is none.
func GetUserToken(r *http.Request) string {
	if token := r.Header.Get(UserTokenHeader); token != "" {
		return token
	}
	if token := r.Header.Get(LegacyUserTokenHeader); token != "" {
		return token
	}

	cookie, err := r.Cookie(userIDCookie)
	if err != nil {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}
	return string(decoded)
}

// SetUserToken stores token in the HttpOnly session cookie.
func SetUserToken(w http.ResponseWriter, token string) {
	setSecureCookie(w, cookieConfig{
		name:     userIDCookie,
		value:    base64.StdEncoding.EncodeToString([]byte(token)),
		path:     "/",
		httpOnly: true,
		maxAge:   int(userIDLifetime.Seconds()),
	})
}
