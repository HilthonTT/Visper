package apisdk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Encrypted message content and reactions are sealed in an envelope that
// names what they are and who wrote them, so the server, which relays the
// sender's user ID in plaintext, cannot pass one member's words off as
// another's or turn a reaction into a message:
//
//	{"v":1,"k":"m"|"r","f":"<sender user ID>","m":"<message ID>","t":"<text>"}
//
// "m" binds a reaction to the message it reacts to, and an edit to the
// message it edits; a new message has none (its ID is assigned by the
// server). The envelope is encrypted with the room key like before
// (EncryptWithKeyB64), and the web client builds the same JSON.
const (
	envelopeVersion = 1
	kindMessage     = "m"
	kindReaction    = "r"
)

// MaxSealedBytes is the largest envelope whose ciphertext fits the API's
// 1000-character limit on message content: base64(nonce || secretbox(env)).
const MaxSealedBytes = 1000/4*3 - NonceSize - secretboxOverhead

const secretboxOverhead = 16

// ErrUnverified means content decrypted with the room key but does not carry
// the sender, kind or message it was presented with.
var ErrUnverified = errors.New("content does not match its sender or message")

// ErrNoSender is returned when encrypting without a sender ID set.
var ErrNoSender = errors.New("encrypted messages need the sender's user ID (MessageService.SetSender)")

type envelope struct {
	V    int    `json:"v"`
	Kind string `json:"k"`
	From string `json:"f"`
	Msg  string `json:"m,omitempty"`
	Text string `json:"t"`
}

func marshalEnvelope(kind, from, messageID, text string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep the size close to the text's
	// Encoding a struct of strings cannot fail.
	_ = enc.Encode(envelope{V: envelopeVersion, Kind: kind, From: from, Msg: messageID, Text: text})
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// SealedMessageSize returns the size of the envelope that sending (or, with
// messageID, editing) text as from would encrypt. It must not exceed
// MaxSealedBytes.
func SealedMessageSize(from, messageID, text string) int {
	return len(marshalEnvelope(kindMessage, from, messageID, text))
}

// SealMessage encrypts message text written by from. Pass the message's ID
// when editing it, "" for a new message.
func SealMessage(text, from, messageID, keyB64 string) (string, error) {
	return seal(kindMessage, from, messageID, text, keyB64)
}

// SealReaction encrypts emoji, from's reaction to messageID.
func SealReaction(emoji, from, messageID, keyB64 string) (string, error) {
	return seal(kindReaction, from, messageID, emoji, keyB64)
}

func seal(kind, from, messageID, text, keyB64 string) (string, error) {
	if from == "" {
		return "", ErrNoSender
	}
	env := marshalEnvelope(kind, from, messageID, text)
	if len(env) > MaxSealedBytes {
		return "", fmt.Errorf("message too long: %d bytes sealed, at most %d", len(env), MaxSealedBytes)
	}
	return EncryptWithKeyB64(string(env), keyB64)
}

// OpenMessage decrypts the content of message messageID, shown as sent by
// from, and checks that from wrote it for that message (or as a new message).
func OpenMessage(ciphertext, keyB64, from, messageID string) (string, error) {
	env, err := open(ciphertext, keyB64)
	if err != nil {
		return "", err
	}
	if env.Kind != kindMessage || env.From != from || (env.Msg != "" && env.Msg != messageID) {
		return "", ErrUnverified
	}
	return env.Text, nil
}

// OpenEdit decrypts an edit of message messageID and checks that from sealed
// it for that message (edits always name the message they edit).
func OpenEdit(ciphertext, keyB64, from, messageID string) (string, error) {
	env, err := open(ciphertext, keyB64)
	if err != nil {
		return "", err
	}
	if env.Kind != kindMessage || env.From != from || env.Msg != messageID {
		return "", ErrUnverified
	}
	return env.Text, nil
}

// OpenReaction decrypts from's reaction to messageID and checks that it is one.
func OpenReaction(ciphertext, keyB64, from, messageID string) (string, error) {
	env, err := open(ciphertext, keyB64)
	if err != nil {
		return "", err
	}
	if env.Kind != kindReaction || env.From != from || env.Msg != messageID {
		return "", ErrUnverified
	}
	return env.Text, nil
}

func open(ciphertext, keyB64 string) (*envelope, error) {
	plain, err := DecryptWithKeyB64(ciphertext, keyB64)
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal([]byte(plain), &env); err != nil || env.V != envelopeVersion || env.From == "" {
		return nil, ErrUnverified // e.g. content from a client without envelopes
	}
	return &env, nil
}
