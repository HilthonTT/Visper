package apisdk

import "testing"

func TestUserIDFromTokenMatchesTheAPI(t *testing.T) {
	// Same vector as services/api/infrastructure/security/user_token_test.go.
	const want = "629bfa91-c2d7-8ec5-b29e-d701d23d339d"
	if got := UserIDFromToken("example-token-0123456789"); got != want {
		t.Fatalf("UserIDFromToken = %q, want %q", got, want)
	}
	if tok := NewUserToken(); len(tok) < MinUserTokenLength {
		t.Fatalf("NewUserToken() = %q is shorter than %d", tok, MinUserTokenLength)
	}
}
