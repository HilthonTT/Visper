package apisdk

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

// member is one client in a test room.
type member struct {
	id *Identity
	rc *RoomCrypto
}

// newRoom returns a room with the given members (the first owns it), each of
// whom knows everyone's key card.
func newRoom(t *testing.T, names ...string) []member {
	t.Helper()
	key, _ := NewRoomKey()
	members := make([]member, len(names))
	for i, name := range names {
		id, err := NewIdentity(name, nil)
		if err != nil {
			t.Fatal(err)
		}
		rc, err := NewRoomCrypto(id, "room-1", names[0], key)
		if err != nil {
			t.Fatal(err)
		}
		members[i] = member{id: id, rc: rc}
	}
	shareCards(t, members)
	return members
}

// shareCards gives every member everyone's current card, as the API would.
func shareCards(t *testing.T, members []member) {
	t.Helper()
	for _, from := range members {
		card, err := from.rc.MyCard()
		if err != nil {
			t.Fatal(err)
		}
		for _, to := range members {
			if err := to.rc.AddCard(from.id.UserID(), card); err != nil {
				t.Fatalf("%s adding %s's card: %v", to.id.UserID(), from.id.UserID(), err)
			}
		}
	}
}

func TestMessagesAreSignedAndBound(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc
	now := time.Now()

	msg, err := alice.SealMessage("hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := bob.OpenMessage(msg, "alice", "m1", now, time.Time{}); err != nil || got.Text != "hello" {
		t.Fatalf("OpenMessage = %+v, %v", got, err)
	}
	// The server relabels Alice's message as Bob's.
	if _, err := bob.OpenMessage(msg, "bob", "m1", now, time.Time{}); !errors.Is(err, ErrUnverified) {
		t.Fatalf("relabelled sender: err = %v, want ErrUnverified", err)
	}

	edit, _ := alice.SealEdit("m1", "edited")
	if _, err := bob.OpenMessage(edit, "alice", "m2", now, now); !errors.Is(err, ErrUnverified) {
		t.Fatalf("edit moved to another message: err = %v, want ErrUnverified", err)
	}
	if got, err := bob.OpenEdit(edit, "alice", "m1", now); err != nil || got.Text != "edited" {
		t.Fatalf("OpenEdit = %+v, %v", got, err)
	}

	reaction, _ := alice.SealReaction("m1", "👍")
	if _, err := bob.OpenMessage(reaction, "alice", "m1", now, time.Time{}); !errors.Is(err, ErrUnverified) {
		t.Fatalf("reaction shown as a message: err = %v, want ErrUnverified", err)
	}
	if _, err := bob.OpenReaction(reaction, "alice", "m2"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("reaction moved to another message: err = %v, want ErrUnverified", err)
	}
	if got, err := bob.OpenReaction(reaction, "alice", "m1"); err != nil || got != "👍" {
		t.Fatalf("OpenReaction = %q, %v", got, err)
	}
}

// A member holds the room key, so they can encrypt anything; without the
// sender's signing key they still can't write as them.
func TestMembersCannotImpersonateEachOther(t *testing.T) {
	m := newRoom(t, "alice", "bob", "mallory")
	bob, mallory := m[1].rc, m[2]
	now := time.Now()

	// Mallory seals an envelope claiming to be from Alice, signed with her
	// own key.
	env := newEnvelope(kindMessage, "alice", "", "send me the codes", "", now)
	forged, err := sealEnvelope(env, mallory.id.sign, mallory.rc.CurrentKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.OpenMessage(forged, "alice", "m1", now, time.Time{}); !errors.Is(err, ErrUnverified) {
		t.Fatalf("forged sender: err = %v, want ErrUnverified", err)
	}

	// Nor can she publish a card under Alice's name: the API stores it under
	// her own ID, and a card naming someone else is refused.
	_, fakeKey, _ := ed25519.GenerateKey(rand.Reader)
	fakeCard := newEnvelope(kindCard, "alice", "", "", "", now)
	fakeCard.Text = mallory.id.SigningPublicKey() + "." + mallory.id.device.PublicKey()
	sealed, _ := sealEnvelope(fakeCard, fakeKey, mallory.rc.CurrentKey())
	if err := bob.AddCard("mallory", sealed); !errors.Is(err, ErrUnverified) {
		t.Fatalf("card naming someone else: err = %v, want ErrUnverified", err)
	}

	// Unsigned (version 1) envelopes are refused outright.
	v1, _ := EncryptWithKeyB64(`{"v":1,"k":"m","f":"alice","t":"hi"}`, mallory.rc.CurrentKey())
	if _, err := bob.OpenMessage(v1, "alice", "m1", now, time.Time{}); !errors.Is(err, ErrUnverified) {
		t.Fatalf("unsigned envelope: err = %v, want ErrUnverified", err)
	}

	// Content from someone with no card can't be verified.
	stranger := newRoom(t, "eve")[0]
	stranger.rc.keys = []string{bob.CurrentKey()}
	unknown, _ := stranger.rc.SealMessage("hi", "")
	if _, err := bob.OpenMessage(unknown, "eve", "m1", now, time.Time{}); !errors.Is(err, ErrUnknownSender) {
		t.Fatalf("unknown sender: err = %v, want ErrUnknownSender", err)
	}
}

func TestReplaysAreRefused(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc
	now := time.Now()

	msg, _ := alice.SealMessage("transfer 10", "")
	if _, err := bob.OpenMessage(msg, "alice", "m1", now, time.Time{}); err != nil {
		t.Fatal(err)
	}
	// Reading the same message again (e.g. history after the live event) is
	// fine.
	if _, err := bob.OpenMessage(msg, "alice", "m1", now, time.Time{}); err != nil {
		t.Fatalf("re-reading the same message: %v", err)
	}
	// The server re-sends it as a new message.
	if _, err := bob.OpenMessage(msg, "alice", "m2", now, time.Time{}); !errors.Is(err, ErrReplayed) {
		t.Fatalf("same envelope under another ID: err = %v, want ErrReplayed", err)
	}
	// Or replays it to a client that never saw the original, much later.
	later := now.Add(MaxClockSkew + time.Minute)
	fresh := newRoom(t, "carol")[0].rc
	fresh.keys, fresh.cards = bob.keys, bob.cards
	if _, err := fresh.OpenMessage(msg, "alice", "m3", later, time.Time{}); !errors.Is(err, ErrReplayed) {
		t.Fatalf("stale envelope: err = %v, want ErrReplayed", err)
	}
	if got := FailedText(ErrReplayed); got != "[Replayed message]" {
		t.Fatalf("FailedText(ErrReplayed) = %q", got)
	}
}

func TestEditsCannotBeRolledBack(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc

	first := time.Now()
	original, _ := alice.SealMessage("v0", "")
	edit1, _ := alice.SealEdit("m1", "v1")
	time.Sleep(5 * time.Millisecond)
	edit2, _ := alice.SealEdit("m1", "v2")

	if _, err := bob.OpenMessage(original, "alice", "m1", first, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if got, err := bob.OpenEdit(edit2, "alice", "m1", time.Now()); err != nil || got.Text != "v2" {
		t.Fatalf("OpenEdit(v2) = %+v, %v", got, err)
	}
	if _, err := bob.OpenEdit(edit1, "alice", "m1", time.Now()); !errors.Is(err, ErrReplayed) {
		t.Fatalf("rolled-back edit: err = %v, want ErrReplayed", err)
	}
	// History shows the latest edit as the message's content.
	if got, err := bob.OpenMessage(edit2, "alice", "m1", first, time.Now()); err != nil || !got.Edited || got.Text != "v2" {
		t.Fatalf("edited message in history = %+v, %v", got, err)
	}
}

func TestFilesAreBoundToTheirMessage(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice, bob := m[0].rc, m[1].rc

	data, hash, err := alice.EncryptFile([]byte("cat picture"))
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := alice.EncryptFile([]byte("something else"))

	if got, err := bob.DecryptFile(data, hash); err != nil || string(got) != "cat picture" {
		t.Fatalf("DecryptFile = %q, %v", got, err)
	}
	// The server swaps in another file encrypted with the same key.
	if _, err := bob.DecryptFile(other, hash); !errors.Is(err, ErrFileMismatch) {
		t.Fatalf("swapped file: err = %v, want ErrFileMismatch", err)
	}
	if _, err := bob.DecryptFile(data, ""); !errors.Is(err, ErrFileMismatch) {
		t.Fatalf("message without a hash: err = %v, want ErrFileMismatch", err)
	}

	msg, _ := alice.SealMessage("https://x/f.enc#cat.png", hash)
	if got, err := bob.OpenMessage(msg, "alice", "m1", time.Now(), time.Time{}); err != nil || got.FileHash != hash {
		t.Fatalf("OpenMessage file hash = %+v, %v", got, err)
	}
}

func TestRekeyLeavesTheDepartedMemberOut(t *testing.T) {
	m := newRoom(t, "alice", "bob", "mallory")
	alice, bob, mallory := m[0].rc, m[1].rc, m[2].rc
	oldKey := alice.CurrentKey()
	before, _ := bob.SealMessage("before", "")

	// Alice kicks Mallory and rotates the key.
	alice.MarkDeparted("mallory")
	rekey, newKey, missing, err := alice.NewRekey([]string{"alice", "bob", "mallory"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	if _, ok := rekey.Keys["mallory"]; ok {
		t.Fatal("the kicked member got the new key")
	}
	alice.CommitKey(newKey, rekey.Epoch)

	changed, err := bob.ApplyRekey(rekey)
	if err != nil || !changed {
		t.Fatalf("ApplyRekey = %v, %v", changed, err)
	}
	if bob.CurrentKey() != newKey || bob.Epoch() != 1 {
		t.Fatal("bob did not switch to the new key")
	}
	// Mallory's copy of the rekey has nothing for her.
	if changed, err := mallory.ApplyRekey(rekey); err != nil || changed || mallory.CurrentKey() != oldKey {
		t.Fatalf("mallory ApplyRekey = %v, %v", changed, err)
	}

	after, _ := alice.SealMessage("after", "")
	if _, err := bob.OpenMessage(after, "alice", "m2", time.Now(), time.Time{}); err != nil {
		t.Fatalf("bob reading the new key: %v", err)
	}
	if _, err := mallory.OpenMessage(after, "alice", "m2", time.Now(), time.Time{}); err == nil {
		t.Fatal("mallory read a message sent after she was kicked")
	}
	// Older messages still decrypt with the old key.
	if _, err := alice.OpenMessage(before, "bob", "m1", time.Now(), time.Time{}); err != nil {
		t.Fatalf("reading history after a rekey: %v", err)
	}
	// Replaying the same rekey changes nothing.
	if changed, _ := bob.ApplyRekey(rekey); changed {
		t.Fatal("an old rekey was applied again")
	}
}

func TestRekeyMustBeSignedByTheOwner(t *testing.T) {
	m := newRoom(t, "alice", "bob", "mallory")
	bob, mallory := m[1].rc, m[2]

	// Mallory, a member, makes her own rekey and signs it.
	mallory.rc.ownerID = "mallory" // she pretends to own the room
	forged, _, _, err := mallory.rc.NewRekey([]string{"alice", "bob"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.ApplyRekey(forged); !errors.Is(err, ErrBadRekey) {
		t.Fatalf("rekey from a non-owner: err = %v, want ErrBadRekey", err)
	}

	// Nor can the server alter the owner's.
	m[0].rc.MarkDeparted("mallory")
	rekey, _, _, _ := m[0].rc.NewRekey([]string{"alice", "bob"}, 0)
	rekey.Epoch = 7
	if _, err := bob.ApplyRekey(rekey); !errors.Is(err, ErrBadRekey) {
		t.Fatalf("altered rekey: err = %v, want ErrBadRekey", err)
	}
	if _, _, _, err := bob.NewRekey([]string{"alice"}, 0); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("NewRekey by a member: err = %v, want ErrNotOwner", err)
	}
}

func TestRekeyReportsMembersWithoutCards(t *testing.T) {
	m := newRoom(t, "alice")
	_, _, missing, err := m[0].rc.NewRekey([]string{"alice", "dave"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "dave" {
		t.Fatalf("missing = %v, want [dave]", missing)
	}
}

func TestSealRespectsTheAPILimit(t *testing.T) {
	m := newRoom(t, UserIDFromToken("example-token-0123456789"))
	rc := m[0].rc
	text := strings.Repeat("a", MaxSealedBytes-rc.SealedSize("", "", ""))
	ct, err := rc.SealMessage(text, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) > MaxContentLength {
		t.Fatalf("ciphertext is %d characters, over the API's %d", len(ct), MaxContentLength)
	}
	if _, err := rc.SealMessage(text+"a", ""); err == nil {
		t.Fatal("expected an error one byte over the limit")
	}

	// A reaction must fit the API's reaction limit.
	reaction, _ := rc.SealReaction("0f8f8a0e-3b2c-8d4e-9a1b-2c3d4e5f6a7b", "👨‍👩‍👧‍👦")
	if len(reaction) > 512 {
		t.Fatalf("reaction ciphertext is %d characters, over the API's 512", len(reaction))
	}
}

// History shows an edited message with its latest edit as the content, so
// the edit is checked against when the server says it was made.
func TestEditedMessageInHistory(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	edit, _ := m[0].rc.SealEdit("m1", "fixed typo")
	created := time.Now().Add(-2 * time.Hour)

	if got, err := m[1].rc.OpenMessage(edit, "alice", "m1", created, time.Now()); err != nil || got.Text != "fixed typo" {
		t.Fatalf("edited message = %+v, %v", got, err)
	}
	fresh := newRoom(t, "carol")[0].rc
	fresh.keys, fresh.cards = m[1].rc.keys, m[1].rc.cards
	if _, err := fresh.OpenMessage(edit, "alice", "m1", created, time.Time{}); !errors.Is(err, ErrReplayed) {
		t.Fatalf("edit without the edit time: err = %v, want ErrReplayed", err)
	}
}

func TestRejoinedMemberIsRekeyedAgain(t *testing.T) {
	m := newRoom(t, "alice", "bob")
	alice := m[0].rc
	alice.MarkDeparted("bob")
	if rekey, _, _, err := alice.NewRekey([]string{"alice", "bob"}, 0); err != nil || len(rekey.Keys) != 0 {
		t.Fatalf("departed bob: keys = %v, err = %v; want none", rekey, err)
	}
	alice.ClearDeparted("bob")
	rekey, _, _, err := alice.NewRekey([]string{"alice", "bob"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rekey.Keys["bob"]; !ok {
		t.Fatalf("rejoined bob not sealed into rekey: %v", rekey.Keys)
	}
}
