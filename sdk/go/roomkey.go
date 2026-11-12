package apisdk

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/nacl/secretbox"
)

// Room keys are created by the client that creates a room and are never sent
// to the API. Everyone else gets the key out of band, inside the invite:
//
//   - an invite code, "<join code>.<key>", to paste into a client;
//   - an invite link, whose "#key=<key>" fragment browsers never send to a
//     server;
//   - a sealed key, the room key encrypted to one device's public key, which
//     the API relays without being able to read it.
//
// In invites the key is unpadded base64url. Elsewhere in this SDK (e.g.
// MessageService.SetEncryptionKey) it is standard base64, as returned by
// NewRoomKey.

// separator joins the parts of invite and device codes. It is neither in
// base64url, in join codes nor in user IDs (UUIDs).
const codeSeparator = "."

// roomKeyFragmentParam is the invite-link fragment parameter holding the key.
const roomKeyFragmentParam = "key"

var (
	ErrMissingRoomKey   = errors.New("the invite code has no room key")
	ErrInvalidRoomKey   = errors.New("invalid room key")
	ErrInvalidDeviceKey = errors.New("invalid device code")
	ErrInvalidSealedKey = errors.New("invalid sealed room key")
)

// NewRoomKey returns a new random room key, base64 encoded.
func NewRoomKey() (string, error) {
	return GenerateKeyBase64()
}

// roomKeyBytes decodes and checks a base64 room key.
func roomKeyBytes(keyB64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != KeySize {
		return nil, ErrInvalidRoomKey
	}
	return key, nil
}

// RoomKeyToURL encodes a base64 room key for an invite (base64url).
func RoomKeyToURL(keyB64 string) (string, error) {
	key, err := roomKeyBytes(keyB64)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}

// RoomKeyFromURL decodes a room key from an invite into base64.
func RoomKeyFromURL(s string) (string, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil || len(key) != KeySize {
		return "", ErrInvalidRoomKey
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// FormatInviteCode returns the code to share for a room: its join code and
// its key.
func FormatInviteCode(joinCode, keyB64 string) (string, error) {
	key, err := RoomKeyToURL(keyB64)
	if err != nil {
		return "", err
	}
	return joinCode + codeSeparator + key, nil
}

// ParseInviteCode splits an invite code into its join code and base64 room
// key. A bare join code returns ErrMissingRoomKey.
func ParseInviteCode(code string) (joinCode, keyB64 string, err error) {
	code = strings.TrimSpace(code)
	joinCode, key, ok := strings.Cut(code, codeSeparator)
	if !ok || key == "" {
		return strings.ToUpper(joinCode), "", ErrMissingRoomKey
	}
	keyB64, err = RoomKeyFromURL(key)
	if err != nil {
		return "", "", err
	}
	return strings.ToUpper(joinCode), keyB64, nil
}

// WithRoomKeyFragment returns the invite link inviteURL carrying the room key
// in its fragment.
func WithRoomKeyFragment(inviteURL, keyB64 string) (string, error) {
	key, err := RoomKeyToURL(keyB64)
	if err != nil {
		return "", err
	}
	base, _, _ := strings.Cut(inviteURL, "#")
	return base + "#" + roomKeyFragmentParam + "=" + key, nil
}

// ---------------------------------------------------------------------------
// Device keys and sealed room keys
//
// A device key lets a browser hand a room key to a CLI through the API (the
// room invite notification) without the API learning it. The sealed key is
//
//	base64url( ephemeral X25519 public key (32) || nonce (12) || AES-256-GCM ciphertext )
//
// where the AES key is HKDF-SHA256 of the X25519 shared secret, salted with
// both public keys. The web client implements the same scheme with WebCrypto.

const sealInfo = "visper/sealed-room-key/v1"

// DeviceKey is an X25519 key pair identifying one running client.
type DeviceKey struct {
	priv *ecdh.PrivateKey
}

// NewDeviceKey returns a new random device key.
func NewDeviceKey() (*DeviceKey, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device key: %w", err)
	}
	return &DeviceKey{priv: priv}, nil
}

// PublicKey returns the device's public key, base64url encoded.
func (k *DeviceKey) PublicKey() string {
	return base64.RawURLEncoding.EncodeToString(k.priv.PublicKey().Bytes())
}

// FormatDeviceCode returns the code a user copies into the web client to send
// invites to this device: the user ID and the device's public key.
func FormatDeviceCode(userID string, k *DeviceKey) string {
	return userID + codeSeparator + k.PublicKey()
}

// ParseDeviceCode splits a device code into its user ID and public key.
func ParseDeviceCode(code string) (userID, publicKey string, err error) {
	userID, publicKey, ok := strings.Cut(strings.TrimSpace(code), codeSeparator)
	if !ok || userID == "" {
		return "", "", ErrInvalidDeviceKey
	}
	if _, err := parsePublicKey(publicKey); err != nil {
		return "", "", err
	}
	return userID, publicKey, nil
}

func parsePublicKey(s string) (*ecdh.PublicKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrInvalidDeviceKey
	}
	pub, err := ecdh.X25519().NewPublicKey(raw)
	if err != nil {
		return nil, ErrInvalidDeviceKey
	}
	return pub, nil
}

// sealCipher derives the AES-GCM cipher for one sealed key.
func sealCipher(shared, ephemeralPub, recipientPub []byte) (cipher.AEAD, error) {
	salt := append(append([]byte{}, ephemeralPub...), recipientPub...)
	key, err := hkdf.Key(sha256.New, shared, salt, sealInfo, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealRoomKey encrypts a base64 room key to the device whose base64url public
// key is recipientPublicKey.
func SealRoomKey(keyB64, recipientPublicKey string) (string, error) {
	roomKey, err := roomKeyBytes(keyB64)
	if err != nil {
		return "", err
	}
	recipient, err := parsePublicKey(recipientPublicKey)
	if err != nil {
		return "", err
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	shared, err := ephemeral.ECDH(recipient)
	if err != nil {
		return "", ErrInvalidDeviceKey
	}
	ephemeralPub := ephemeral.PublicKey().Bytes()
	aead, err := sealCipher(shared, ephemeralPub, recipient.Bytes())
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	_, _ = rand.Read(nonce)

	out := append(append([]byte{}, ephemeralPub...), nonce...)
	out = aead.Seal(out, nonce, roomKey, nil)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// OpenRoomKey decrypts a key sealed to this device and returns it base64
// encoded.
func (k *DeviceKey) OpenRoomKey(sealed string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(sealed, "="))
	if err != nil || len(data) < 32+12+KeySize+16 {
		return "", ErrInvalidSealedKey
	}
	ephemeralPub, nonce, ciphertext := data[:32], data[32:44], data[44:]
	ephemeral, err := ecdh.X25519().NewPublicKey(ephemeralPub)
	if err != nil {
		return "", ErrInvalidSealedKey
	}
	shared, err := k.priv.ECDH(ephemeral)
	if err != nil {
		return "", ErrInvalidSealedKey
	}
	aead, err := sealCipher(shared, ephemeralPub, k.priv.PublicKey().Bytes())
	if err != nil {
		return "", err
	}
	roomKey, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil || len(roomKey) != KeySize {
		return "", ErrInvalidSealedKey
	}
	return base64.StdEncoding.EncodeToString(roomKey), nil
}

// ---------------------------------------------------------------------------
// Files

// encryptedFileMagic starts every encrypted file. The API only accepts uploads
// that start with it. The NUL byte keeps content sniffing from ever taking
// the file for text.
var encryptedFileMagic = []byte("VISPER\x00\x01")

// encryptedFileOverhead is how many bytes encryption adds to a file.
const encryptedFileOverhead = 8 + NonceSize + 16 // magic + nonce + Poly1305 tag

// EncryptFile encrypts a file's content with a base64 room key.
func EncryptFile(plaintext []byte, keyB64 string) ([]byte, error) {
	key, err := roomKeyBytes(keyB64)
	if err != nil {
		return nil, err
	}
	var k [KeySize]byte
	copy(k[:], key)
	var nonce [NonceSize]byte
	_, _ = rand.Read(nonce[:])

	out := make([]byte, 0, len(plaintext)+encryptedFileOverhead)
	out = append(out, encryptedFileMagic...)
	out = append(out, nonce[:]...)
	return secretbox.Seal(out, plaintext, &nonce, &k), nil
}

// DecryptFile decrypts a file encrypted by EncryptFile.
func DecryptFile(data []byte, keyB64 string) ([]byte, error) {
	key, err := roomKeyBytes(keyB64)
	if err != nil {
		return nil, err
	}
	if !IsEncryptedFile(data) || len(data) < encryptedFileOverhead {
		return nil, ErrInvalidCiphertext
	}
	var k [KeySize]byte
	copy(k[:], key)
	var nonce [NonceSize]byte
	copy(nonce[:], data[len(encryptedFileMagic):])

	plaintext, ok := secretbox.Open(nil, data[len(encryptedFileMagic)+NonceSize:], &nonce, &k)
	if !ok {
		return nil, ErrDecryptionFailed
	}
	return plaintext, nil
}

// IsEncryptedFile reports whether data starts like an encrypted file.
func IsEncryptedFile(data []byte) bool {
	return len(data) >= len(encryptedFileMagic) && string(data[:len(encryptedFileMagic)]) == string(encryptedFileMagic)
}
