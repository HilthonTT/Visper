package apisdk

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty", ""},
		{"ascii", "hello, world"},
		{"unicode", "héllo 🍣 世界"},
		{"long", strings.Repeat("x", 4096)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := Encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			got, err := Decrypt(ct, key)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if got != tt.plaintext {
				t.Fatalf("round trip = %q, want %q", got, tt.plaintext)
			}
		})
	}
}

func TestEncryptUsesFreshNonce(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Encrypt("same", key)
	b, _ := Encrypt("same", key)
	if a == b {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertext (nonce reuse)")
	}
	ra, _ := base64.StdEncoding.DecodeString(a)
	rb, _ := base64.StdEncoding.DecodeString(b)
	if bytes.Equal(ra[:NonceSize], rb[:NonceSize]) {
		t.Fatal("nonce reused")
	}
}

func TestDecryptErrors(t *testing.T) {
	key, _ := GenerateKey()
	otherKey, _ := GenerateKey()
	ct, err := Encrypt("secret", key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(ct)
	tampered := bytes.Clone(raw)
	tampered[len(tampered)-1] ^= 0xff

	tests := []struct {
		name    string
		ct      string
		key     []byte
		wantErr error
	}{
		{"wrong key", ct, otherKey, ErrDecryptionFailed},
		{"tampered", base64.StdEncoding.EncodeToString(tampered), key, ErrDecryptionFailed},
		{"too short", base64.StdEncoding.EncodeToString(raw[:NonceSize-1]), key, ErrInvalidCiphertext},
		{"short key", ct, key[:16], ErrInvalidKey},
		{"bad base64", "!!!", key, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(tt.ct, tt.key)
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestKeyB64Helpers(t *testing.T) {
	keyB64, err := GenerateKeyBase64()
	if err != nil {
		t.Fatal(err)
	}
	ct, err := EncryptWithKeyB64("hi", keyB64)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptWithKeyB64(ct, keyB64)
	if err != nil || got != "hi" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := EncryptWithKeyB64("hi", "not base64!"); err == nil {
		t.Fatal("expected error for invalid key encoding")
	}
	if _, err := Encrypt("hi", make([]byte, 31)); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("err = %v, want ErrInvalidKey", err)
	}
}
