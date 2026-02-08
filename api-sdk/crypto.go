package apisdk

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/secretbox"
)

const (
	KeySize   = 32 // 256-bit key
	NonceSize = 24 // NaCl secretbox nonce size
)

var (
	ErrInvalidKey        = errors.New("invalid key size")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrDecryptionFailed  = errors.New("decryption failed")
)

func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

func GenerateKeyBase64() (string, error) {
	key, err := GenerateKey()
	if err != nil {
		return "", nil
	}

	return base64.StdEncoding.EncodeToString(key), nil
}

func Encrypt(plaintext string, keyBytes []byte) (string, error) {
	if len(keyBytes) != KeySize {
		return "", ErrInvalidKey
	}

	// Convert to fixed-size array
	var key [KeySize]byte
	copy(key[:], keyBytes)

	// Generate random nonce
	var nonce [NonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	encrypted := secretbox.Seal(nonce[:], []byte(plaintext), &nonce, &key)

	// Return base64 encoded
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func Decrypt(ciphertextB64 string, keyBytes []byte) (string, error) {
	if len(keyBytes) != KeySize {
		return "", ErrInvalidKey
	}

	// Convert to fixed-size array
	var key [KeySize]byte
	copy(key[:], keyBytes)

	// Decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("invalid base64: %w", err)
	}

	// Extract nonce
	if len(ciphertext) < NonceSize {
		return "", ErrInvalidCiphertext
	}

	var nonce [NonceSize]byte
	copy(nonce[:], ciphertext[:NonceSize])

	// Decrypt
	decrypted, ok := secretbox.Open(nil, ciphertext[NonceSize:], &nonce, &key)
	if !ok {
		return "", ErrDecryptionFailed
	}

	return string(decrypted), nil
}

// EncryptWithKeyB64 encrypts using a base64-encoded key
func EncryptWithKeyB64(plaintext, keyB64 string) (string, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return "", fmt.Errorf("invalid key: %w", err)
	}
	return Encrypt(plaintext, keyBytes)
}

// DecryptWithKeyB64 decrypts using a base64-encoded key
func DecryptWithKeyB64(ciphertextB64, keyB64 string) (string, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return "", fmt.Errorf("invalid key: %w", err)
	}
	return Decrypt(ciphertextB64, keyBytes)
}
