package apisdk

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Everything a member encrypts (message text, edits, reactions and key
// cards) is a signed envelope, sealed with the room key:
//
//	secretbox( 0x02 || Ed25519 signature (64) || envelope JSON )
//
//	{"v":2,"k":"m"|"r"|"c","f":"<sender user ID>","m":"<message ID>",
//	 "n":"<random nonce>","ts":<unix ms>,"t":"<text>","h":"<file hash>"}
//
// The signature is made with the sender's signing key, which other members
// learn from the sender's key card (see RoomCrypto). The room key keeps the
// server out; the signature keeps members from writing as each other.
//
//   - "m" binds a reaction to the message it reacts to, and an edit to the
//     message it edits; a new message has none (the server assigns its ID).
//   - "n" and "ts" let readers reject a new message the server replays under
//     another ID, or long after it was written.
//   - "h" is the SHA-256 of the encrypted file an image message points to, so
//     the server can't swap the file.
//
// The web client builds the same bytes (clients/web/src/lib/room-crypto.ts).
const (
	envelopeVersion = 2
	sealedVersion   = 0x02
	kindMessage     = "m"
	kindReaction    = "r"
	kindCard        = "c"

	signatureSize = ed25519.SignatureSize
	nonceBytes    = 16
)

// MaxContentLength is the API's limit on the (encrypted) content of a
// message, in characters.
const MaxContentLength = 4000

// MaxSealedBytes is the largest signed envelope whose ciphertext fits
// MaxContentLength: base64(nonce || secretbox(0x02 || signature || JSON)).
const MaxSealedBytes = MaxContentLength/4*3 - NonceSize - secretboxOverhead - 1 - signatureSize

const secretboxOverhead = 16

// MaxClockSkew is how far an envelope's timestamp may be from the time the
// server says it received it.
const MaxClockSkew = 15 * time.Minute

var (
	// ErrUnverified means content decrypted with a room key but was not
	// signed by its claimed sender, or does not carry the sender, kind or
	// message it was presented with.
	ErrUnverified = errors.New("content does not match its sender or message")
	// ErrUnknownSender means there is no key card for the content's sender.
	ErrUnknownSender = fmt.Errorf("%w: no key card for the sender", ErrUnverified)
	// ErrReplayed means the content is genuine but was replayed: under
	// another message ID, long after it was written, or older than what it
	// replaces.
	ErrReplayed = fmt.Errorf("%w: replayed or out-of-date content", ErrUnverified)
)

type envelope struct {
	V        int    `json:"v"`
	Kind     string `json:"k"`
	From     string `json:"f"`
	Msg      string `json:"m,omitempty"`
	Nonce    string `json:"n"`
	TS       int64  `json:"ts"`
	Text     string `json:"t"`
	FileHash string `json:"h,omitempty"`
}

// time returns when the envelope says it was written.
func (e *envelope) time() time.Time {
	return time.UnixMilli(e.TS)
}

func newEnvelope(kind, from, messageID, text, fileHash string, now time.Time) envelope {
	nonce := make([]byte, nonceBytes)
	_, _ = rand.Read(nonce)
	return envelope{
		V:        envelopeVersion,
		Kind:     kind,
		From:     from,
		Msg:      messageID,
		Nonce:    base64.RawURLEncoding.EncodeToString(nonce),
		TS:       now.UnixMilli(),
		Text:     text,
		FileHash: fileHash,
	}
}

func (e *envelope) marshal() []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep the size close to the text's
	// Encoding a struct of strings and numbers cannot fail.
	_ = enc.Encode(e)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// sealEnvelope signs env and encrypts it with keyB64.
func sealEnvelope(env envelope, signer ed25519.PrivateKey, keyB64 string) (string, error) {
	body := env.marshal()
	if len(body) > MaxSealedBytes {
		return "", fmt.Errorf("message too long: %d bytes sealed, at most %d", len(body), MaxSealedBytes)
	}
	plain := make([]byte, 0, 1+signatureSize+len(body))
	plain = append(plain, sealedVersion)
	plain = append(plain, ed25519.Sign(signer, body)...)
	plain = append(plain, body...)
	return EncryptWithKeyB64(string(plain), keyB64)
}

// openEnvelope decrypts a signed envelope with keyB64. It returns the
// envelope and its signature, which the caller must still check.
func openEnvelope(ciphertext, keyB64 string) (*envelope, []byte, []byte, error) {
	plain, err := DecryptWithKeyB64(ciphertext, keyB64)
	if err != nil {
		return nil, nil, nil, err
	}
	raw := []byte(plain)
	// Unsigned (version 1) envelopes are refused: accepting them would let
	// any member drop the signature and write as someone else.
	if len(raw) < 1+signatureSize || raw[0] != sealedVersion {
		return nil, nil, nil, ErrUnverified
	}
	sig, body := raw[1:1+signatureSize], raw[1+signatureSize:]
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil || env.V != envelopeVersion || env.From == "" || env.Nonce == "" {
		return nil, nil, nil, ErrUnverified
	}
	return &env, sig, body, nil
}

// SealedMessageSize returns the size of the envelope that sending (or, with
// messageID, editing) text as from would sign and encrypt. It must not
// exceed MaxSealedBytes.
func SealedMessageSize(from, messageID, text, fileHash string) int {
	env := newEnvelope(kindMessage, from, messageID, text, fileHash, time.Now())
	return len(env.marshal())
}
