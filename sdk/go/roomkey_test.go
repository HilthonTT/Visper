package apisdk

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestInviteCodeRoundTrip(t *testing.T) {
	key, err := NewRoomKey()
	if err != nil {
		t.Fatal(err)
	}
	code, err := FormatInviteCode("ABC123", key)
	if err != nil {
		t.Fatal(err)
	}
	joinCode, got, err := ParseInviteCode("  " + strings.ToLower(code[:6]) + code[6:] + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if joinCode != "ABC123" || got != key {
		t.Fatalf("ParseInviteCode = %q, %q; want ABC123, %q", joinCode, got, key)
	}
}

func TestParseInviteCodeRejectsBareJoinCodeAndBadKeys(t *testing.T) {
	if joinCode, _, err := ParseInviteCode("ABC123"); !errors.Is(err, ErrMissingRoomKey) || joinCode != "ABC123" {
		t.Fatalf("bare join code: %q, %v", joinCode, err)
	}
	for _, code := range []string{"ABC123.", "ABC123.short", "ABC123.!!!!"} {
		if _, _, err := ParseInviteCode(code); err == nil {
			t.Errorf("ParseInviteCode(%q) accepted a bad key", code)
		}
	}
}

func TestWithRoomKeyFragmentReplacesFragment(t *testing.T) {
	key, _ := NewRoomKey()
	urlKey, _ := RoomKeyToURL(key)
	got, err := WithRoomKeyFragment("https://web.test/?joinCode=A&secureCode=B#old", key)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://web.test/?joinCode=A&secureCode=B#key=" + urlKey; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSealedRoomKeyOpensOnlyForItsDevice(t *testing.T) {
	device, err := NewDeviceKey()
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewDeviceKey()
	key, _ := NewRoomKey()

	userID, pub, err := ParseDeviceCode(FormatDeviceCode("user-1", device))
	if err != nil || userID != "user-1" || pub != device.PublicKey() {
		t.Fatalf("ParseDeviceCode = %q, %q, %v", userID, pub, err)
	}

	sealed, err := SealRoomKey(key, pub)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, key) {
		t.Fatal("sealed key contains the room key")
	}
	if got, err := device.OpenRoomKey(sealed); err != nil || got != key {
		t.Fatalf("OpenRoomKey = %q, %v", got, err)
	}
	if _, err := other.OpenRoomKey(sealed); !errors.Is(err, ErrInvalidSealedKey) {
		t.Fatalf("another device opened the key: %v", err)
	}
	tampered := []byte(sealed)
	tampered[len(tampered)-2] ^= 1
	if _, err := device.OpenRoomKey(string(tampered)); err == nil {
		t.Fatal("a tampered sealed key was accepted")
	}
}

func TestParseDeviceCodeRejectsBadCodes(t *testing.T) {
	for _, code := range []string{"", "user-1", ".abc", "user-1.not-a-key"} {
		if _, _, err := ParseDeviceCode(code); err == nil {
			t.Errorf("ParseDeviceCode(%q) succeeded", code)
		}
	}
}

func TestFileEncryptionRoundTrip(t *testing.T) {
	key, _ := NewRoomKey()
	other, _ := NewRoomKey()
	plain := []byte("\x89PNG\r\n\x1a\n image bytes")

	enc, err := EncryptFile(plain, key)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncryptedFile(enc) || len(enc) != len(plain)+encryptedFileOverhead {
		t.Fatalf("unexpected ciphertext layout (%d bytes)", len(enc))
	}
	if got, err := DecryptFile(enc, key); err != nil || string(got) != string(plain) {
		t.Fatalf("DecryptFile = %q, %v", got, err)
	}
	if _, err := DecryptFile(enc, other); !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := DecryptFile(plain, key); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("plaintext input: %v", err)
	}
}

// A key sealed by the web client (clients/web/src/lib/room-key.ts) to the
// device key whose private key is 32 bytes of 7: the two implementations
// must stay compatible.
func TestOpenRoomKeySealedByWebClient(t *testing.T) {
	priv, err := ecdh.X25519().NewPrivateKey(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	device := &DeviceKey{priv: priv}
	const sealed = "Q9vPHP8ZD_PPTwBCqvXbo-zvomhAR6-HQbkeA2_Zql8WmJ-MQWIJ2ARSLAxTgBXzo77IHnDHULdOCIpXHE594uIjKuT9WajIwOU4fnYQLfoJK83MS22JnebncyY"

	got, err := device.OpenRoomKey(sealed)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, KeySize)
	for i := range want {
		want[i] = byte(i)
	}
	if got != base64.StdEncoding.EncodeToString(want) {
		t.Fatalf("OpenRoomKey = %q", got)
	}
}
