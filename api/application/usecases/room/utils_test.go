package room

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateJoinCode(t *testing.T) {
	seen := make(map[string]struct{})
	for range 200 {
		code := generateJoinCode()
		if len(code) != joinCodeLength {
			t.Fatalf("join code %q has length %d, want %d", code, len(code), joinCodeLength)
		}
		for _, r := range code {
			if !strings.ContainsRune(joinCodeCharset, r) {
				t.Fatalf("join code %q contains %q outside the charset", code, r)
			}
		}
		seen[code] = struct{}{}
	}

	// 32^6 possible codes: 200 draws should essentially never collide.
	if len(seen) < 195 {
		t.Fatalf("only %d distinct codes out of 200; generator is not random", len(seen))
	}
}

func TestGenerateSecureCode(t *testing.T) {
	a, b := generateSecureCode(), generateSecureCode()
	if a == b {
		t.Fatal("secure codes must differ")
	}

	raw, err := base64.RawURLEncoding.DecodeString(a)
	if err != nil {
		t.Fatalf("secure code is not raw URL base64: %v", err)
	}
	if len(raw) != secureCodeLength {
		t.Fatalf("secure code decodes to %d bytes, want %d", len(raw), secureCodeLength)
	}
}
