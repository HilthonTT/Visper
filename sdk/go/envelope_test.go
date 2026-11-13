package apisdk

import (
	"errors"
	"strings"
	"testing"
)

func TestEnvelopeBindsSenderKindAndMessage(t *testing.T) {
	key, _ := GenerateKeyBase64()

	msg, err := SealMessage("hello", "alice", "", key)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := OpenMessage(msg, key, "alice", "m1"); err != nil || got != "hello" {
		t.Fatalf("OpenMessage = %q, %v", got, err)
	}
	// The server relabels Alice's message as Bob's.
	if _, err := OpenMessage(msg, key, "bob", "m1"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("relabelled sender: err = %v, want ErrUnverified", err)
	}

	edit, _ := SealMessage("edited", "alice", "m1", key)
	if _, err := OpenMessage(edit, key, "alice", "m2"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("edit moved to another message: err = %v, want ErrUnverified", err)
	}

	reaction, _ := SealReaction("👍", "alice", "m1", key)
	if _, err := OpenMessage(reaction, key, "alice", "m1"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("reaction shown as a message: err = %v, want ErrUnverified", err)
	}
	if _, err := OpenReaction(reaction, key, "alice", "m2"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("reaction moved to another message: err = %v, want ErrUnverified", err)
	}
	if got, err := OpenReaction(reaction, key, "alice", "m1"); err != nil || got != "👍" {
		t.Fatalf("OpenReaction = %q, %v", got, err)
	}

	// Bare ciphertext (no envelope) is not trusted.
	bare, _ := EncryptWithKeyB64("hello", key)
	if _, err := OpenMessage(bare, key, "alice", "m1"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("bare ciphertext: err = %v, want ErrUnverified", err)
	}
}

func TestSealRespectsTheAPILimit(t *testing.T) {
	key, _ := GenerateKeyBase64()
	from := UserIDFromToken("example-token-0123456789")
	text := strings.Repeat("a", MaxSealedBytes-SealedMessageSize(from, "", ""))
	ct, err := SealMessage(text, from, "", key)
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) > 1000 {
		t.Fatalf("ciphertext is %d characters, over the API's 1000", len(ct))
	}
	if _, err := SealMessage(text+"a", from, "", key); err == nil {
		t.Fatal("expected an error one byte over the limit")
	}
	if _, err := SealMessage("hi", "", "", key); !errors.Is(err, ErrNoSender) {
		t.Fatalf("no sender: err = %v, want ErrNoSender", err)
	}
}
