package apisdk

import (
	"errors"
	"testing"
	"time"
)

func TestIdentityExportRestore(t *testing.T) {
	id, err := NewIdentity("alice", nil)
	if err != nil {
		t.Fatal(err)
	}
	device, err := RestoreDeviceKey(id.Device().Export())
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreIdentity("alice", id.ExportSigningKey(), device)
	if err != nil {
		t.Fatal(err)
	}
	if restored.SigningPublicKey() != id.SigningPublicKey() || restored.Device().PublicKey() != id.Device().PublicKey() {
		t.Fatal("restored identity has different keys")
	}

	// A message signed by the restored identity verifies against the
	// original's card: members see the same person.
	key, _ := NewRoomKey()
	before, _ := NewRoomCrypto(id, "room-1", "alice", key)
	after, _ := NewRoomCrypto(restored, "room-1", "alice", key)
	if before.MyFingerprint() != after.MyFingerprint() {
		t.Fatal("fingerprint changed across a restore")
	}
	bob, _ := NewIdentity("bob", nil)
	bobRC, _ := NewRoomCrypto(bob, "room-1", "alice", key)
	if err := bobRC.AddCard("alice", mustCard(t, before)); err != nil {
		t.Fatal(err)
	}
	msg, _ := after.SealMessage("still me", "")
	if _, err := bobRC.OpenMessage(msg, "alice", "m1", time.Now(), time.Time{}); err != nil {
		t.Fatalf("message from the restored identity: %v", err)
	}

	if _, err := RestoreIdentity("alice", "short", device); !errors.Is(err, ErrInvalidSigningKey) {
		t.Fatalf("bad signing key: %v", err)
	}
	if _, err := RestoreIdentity("", id.ExportSigningKey(), device); !errors.Is(err, ErrNoSender) {
		t.Fatalf("no user ID: %v", err)
	}
	if _, err := RestoreDeviceKey("not base64!"); !errors.Is(err, ErrInvalidDeviceKey) {
		t.Fatalf("bad device key: %v", err)
	}
}

func TestRoomCryptoStateRoundTrip(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc
	if err := alice.VerifyMember("bob", bob.MyFingerprint()); err != nil {
		t.Fatal(err)
	}
	oldMsg, _ := bob.SealMessage("before the rekey", "")
	rekey, newKey, _, err := alice.NewRekey([]string{"alice", "bob"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	alice.CommitKey(newKey, rekey.Epoch)

	st := alice.State()
	restored, err := RestoreRoomCrypto(alice.Identity(), "room-1", "alice", st)
	if err != nil {
		t.Fatal(err)
	}
	if restored.CurrentKey() != newKey || restored.Epoch() != 1 {
		t.Fatalf("restored key/epoch = %q/%d", restored.CurrentKey(), restored.Epoch())
	}
	// Cards come from the API again; the pin survives.
	if err := restored.AddCard("bob", mustCard(t, bob)); err != nil {
		t.Fatal(err)
	}
	if restored.KeyStatus("bob") != KeyVerified {
		t.Fatalf("bob's status after restore = %v", restored.KeyStatus("bob"))
	}
	// Older keys still read history.
	if _, err := restored.OpenMessage(oldMsg, "bob", "m1", time.Now(), time.Time{}); err != nil {
		t.Fatalf("history after restore: %v", err)
	}
	// A different card for bob is still caught.
	forged, _ := forgeCard(t, newKey, "alice", "bob")
	_ = restored.AddCard("bob", forged)
	if restored.KeyStatus("bob") != KeyChanged {
		t.Fatal("a new card for a verified member went unnoticed after restore")
	}

	// The saved state is a copy.
	st.Verified["bob"] = "tampered"
	if alice.State().Verified["bob"] == "tampered" {
		t.Fatal("State shares its map")
	}
	if _, err := RestoreRoomCrypto(alice.Identity(), "room-1", "alice", RoomCryptoState{}); err == nil {
		t.Fatal("restored a state without keys")
	}
	if _, err := RestoreRoomCrypto(alice.Identity(), "room-1", "alice", RoomCryptoState{Keys: []string{newKey, "bad"}}); err == nil {
		t.Fatal("restored a state with a bad key")
	}
}
