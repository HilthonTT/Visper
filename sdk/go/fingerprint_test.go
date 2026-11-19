package apisdk

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

// forgeCard returns a key card for victimID made by someone who has the room
// key but not the victim's keys, and that someone's crypto state for it: what
// a departed member working with the server could slip in.
func forgeCard(t *testing.T, roomKey, ownerID, victimID string) (string, *RoomCrypto) {
	t.Helper()
	id, err := NewIdentity(victimID, nil)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := NewRoomCrypto(id, "room-1", ownerID, roomKey)
	if err != nil {
		t.Fatal(err)
	}
	card, err := rc.MyCard()
	if err != nil {
		t.Fatal(err)
	}
	return card, rc
}

func TestFingerprintsMatchAcrossClients(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc

	fp := bob.MyFingerprint()
	if !regexp.MustCompile(`^\d{5}( \d{5}){5}$`).MatchString(fp) {
		t.Fatalf("fingerprint %q is not six groups of five digits", fp)
	}
	if got := alice.Fingerprint("bob"); got != fp {
		t.Fatalf("alice sees %q for bob, bob shows %q", got, fp)
	}
	if alice.Fingerprint("carol") != "" {
		t.Fatal("a fingerprint for a member without a card")
	}
	if alice.MyFingerprint() == fp {
		t.Fatal("two identities share a fingerprint")
	}
	// The user ID is part of it: the same keys for another member differ.
	if KeyFingerprint("carol", m[1].id.SigningPublicKey(), m[1].id.Device().PublicKey()) == fp {
		t.Fatal("the fingerprint doesn't depend on the user ID")
	}
}

func TestVerifyMember(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc

	if alice.KeyStatus("bob") != KeyUnverified {
		t.Fatal("bob starts verified")
	}
	if err := alice.VerifyMember("bob", "00000 00000 00000 00000 00000 00000"); !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("wrong fingerprint: err = %v", err)
	}
	// Typed without spaces, as someone might read it out.
	digits := regexp.MustCompile(`\D`).ReplaceAllString(bob.MyFingerprint(), "")
	if err := alice.VerifyMember("bob", digits); err != nil {
		t.Fatal(err)
	}
	if alice.KeyStatus("bob") != KeyVerified {
		t.Fatal("bob is not verified")
	}
	// Bob's own card coming round again changes nothing.
	card, _ := bob.MyCard()
	_ = alice.AddCard("bob", card)
	if alice.KeyStatus("bob") != KeyVerified {
		t.Fatal("re-uploading the pinned card counted as a change")
	}
	alice.UnverifyMember("bob")
	if alice.KeyStatus("bob") != KeyUnverified {
		t.Fatal("bob is still verified")
	}
}

// The attack docs/security-model.md describes: a departed member, with the
// server's help, adds a card in a remaining member's name so the owner seals
// the next key to them. Once the owner has verified that member, it fails.
func TestVerifiedMemberCannotBeReplaced(t *testing.T) {
	m := newRoom(t, "alice", "bob", "mallory")
	alice, bob := m[0].rc, m[1].rc
	if err := alice.VerifyMember("bob", bob.MyFingerprint()); err != nil {
		t.Fatal(err)
	}

	forged, fake := forgeCard(t, alice.CurrentKey(), "alice", "bob")
	if err := alice.AddCard("bob", forged); err != nil {
		t.Fatal(err)
	}
	if alice.KeyStatus("bob") != KeyChanged {
		t.Fatal("the new card for bob was not flagged")
	}
	if alice.Fingerprint("bob") == bob.MyFingerprint() {
		t.Fatal("the changed key shows bob's real fingerprint")
	}

	// Messages signed with the forged card are refused...
	msg, _ := fake.SealMessage("hi, it's bob", "")
	if _, err := alice.OpenMessage(msg, "bob", "m1", time.Now(), time.Time{}); !errors.Is(err, ErrUnverified) {
		t.Fatalf("message from the forged card: err = %v, want ErrUnverified", err)
	}
	// ...while bob's real ones still verify.
	msg, _ = bob.SealMessage("the real bob", "")
	if _, err := alice.OpenMessage(msg, "bob", "m2", time.Now(), time.Time{}); err != nil {
		t.Fatalf("message from bob: %v", err)
	}

	// The rekey after Mallory leaves goes to bob's real device only.
	alice.MarkDeparted("mallory")
	rekey, newKey, _, err := alice.NewRekey([]string{"alice", "bob"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fake.Identity().Device().OpenRoomKey(rekey.Keys["bob"]); err == nil {
		t.Fatal("the forged card's device got the new key")
	}
	_ = bob.AddCard("alice", mustCard(t, alice))
	if changed, err := bob.ApplyRekey(rekey); err != nil || !changed || bob.CurrentKey() != newKey {
		t.Fatalf("bob ApplyRekey = %v, %v", changed, err)
	}

	// Without verification, the latest card wins: the attack the pin stops.
	alice.UnverifyMember("bob")
	rekey, _, _, _ = alice.NewRekey([]string{"alice", "bob"}, 1)
	if _, err := fake.Identity().Device().OpenRoomKey(rekey.Keys["bob"]); err != nil {
		t.Fatalf("expected the unpinned rekey to use the latest card: %v", err)
	}

	// Re-verifying with what bob reads out restores the pin.
	if err := alice.VerifyMember("bob", bob.MyFingerprint()); err != nil || alice.KeyStatus("bob") != KeyVerified {
		t.Fatalf("re-verify = %v, status %v", err, alice.KeyStatus("bob"))
	}
}

func TestVerifiedOwnerPinsRekeys(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc
	if err := bob.VerifyMember("alice", alice.MyFingerprint()); err != nil {
		t.Fatal(err)
	}

	// Someone with the room key forges a card for the owner and signs a
	// rekey with it.
	forged, fake := forgeCard(t, alice.CurrentKey(), "alice", "alice")
	if err := bob.AddCard("alice", forged); err != nil {
		t.Fatal(err)
	}
	_ = fake.AddCard("bob", mustCard(t, bob))
	rekey, _, _, err := fake.NewRekey([]string{"alice", "bob"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.ApplyRekey(rekey); !errors.Is(err, ErrBadRekey) {
		t.Fatalf("rekey signed by a forged owner card: err = %v, want ErrBadRekey", err)
	}
	if bob.KeyStatus("alice") != KeyChanged {
		t.Fatal("the forged owner card was not flagged")
	}
}

func mustCard(t *testing.T, rc *RoomCrypto) string {
	t.Helper()
	card, err := rc.MyCard()
	if err != nil {
		t.Fatal(err)
	}
	return card
}
