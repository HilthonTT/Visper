package security

import (
	"net/http/httptest"
	"testing"
)

func TestUserIDFromToken(t *testing.T) {
	// Shared vector: the Go SDK and the AI service must derive the same ID.
	const want = "629bfa91-c2d7-8ec5-b29e-d701d23d339d"
	if got := UserIDFromToken("example-token-0123456789"); got != want {
		t.Fatalf("UserIDFromToken = %q, want %q", got, want)
	}
	if UserIDFromToken("a-different-token-xyz") == want {
		t.Fatal("different tokens must give different IDs")
	}
}

func TestUserTokenRoundTripsThroughCookie(t *testing.T) {
	token := NewUserToken()
	if !ValidUserToken(token) {
		t.Fatalf("generated token %q is not valid", token)
	}

	rec := httptest.NewRecorder()
	SetUserToken(rec, token)
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if got := GetUserToken(req); got != token {
		t.Fatalf("GetUserToken = %q, want %q", got, token)
	}

	req.Header.Set(UserTokenHeader, "from-the-header-0123")
	if got := GetUserToken(req); got != "from-the-header-0123" {
		t.Fatalf("header should win over the cookie, got %q", got)
	}
}

func TestValidUserTokenRejectsShortTokens(t *testing.T) {
	if ValidUserToken("bob") {
		t.Fatal("a 3-character token must be rejected")
	}
}
