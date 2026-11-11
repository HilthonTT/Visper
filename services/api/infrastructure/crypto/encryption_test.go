package crypto

import (
	"encoding/base64"
	"errors"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	keyB64, err := GenerateKeyBase64()
	if err != nil {
		t.Fatalf("GenerateKeyBase64: %v", err)
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != KeySize {
		t.Fatalf("generated key is not %d base64 bytes: %v", KeySize, err)
	}

	for _, plaintext := range []string{"", "hello", "üñïçødé 🔐"} {
		ct, err := EncryptWithKeyB64(plaintext, keyB64)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plaintext, err)
		}

		ct2, _ := EncryptWithKeyB64(plaintext, keyB64)
		if ct == ct2 {
			t.Fatalf("Encrypt(%q) produced identical ciphertexts; nonce is not random", plaintext)
		}

		got, err := DecryptWithKeyB64(ct, keyB64)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != plaintext {
			t.Fatalf("round trip = %q, want %q", got, plaintext)
		}
	}
}

func TestDecryptErrors(t *testing.T) {
	key, _ := GenerateKey()
	other, _ := GenerateKey()

	ct, err := Encrypt("secret", key)
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := base64.StdEncoding.DecodeString(ct)
	raw[len(raw)-1] ^= 0xff
	tampered := base64.StdEncoding.EncodeToString(raw)

	tests := []struct {
		name    string
		ct      string
		key     []byte
		wantErr error
	}{
		{"wrong key", ct, other, ErrDecryptionFailed},
		{"tampered ciphertext", tampered, key, ErrDecryptionFailed},
		{"too short", base64.StdEncoding.EncodeToString([]byte("short")), key, ErrInvalidCiphertext},
		{"bad key size", ct, key[:16], ErrInvalidKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decrypt(tt.ct, tt.key); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Decrypt err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
