package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/resource"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// peer is another member of a test room: their own identity and crypto state.
func newPeer(t *testing.T, userID, roomID, ownerID, key string) *apisdk.RoomCrypto {
	t.Helper()
	id, err := apisdk.NewIdentity(userID, nil)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := apisdk.NewRoomCrypto(id, roomID, ownerID, key)
	if err != nil {
		t.Fatal(err)
	}
	return rc
}

// cardEvent is the MemberCard event the API relays for rc's card.
func cardEvent(t *testing.T, rc *apisdk.RoomCrypto) apisdk.WSMessage {
	t.Helper()
	card, err := rc.MyCard()
	if err != nil {
		t.Fatal(err)
	}
	return apisdk.WSMessage{Type: apisdk.MemberCard, Data: map[string]any{"userId": rc.Identity().UserID(), "card": card}}
}

// exchangeCards gives each of rcs everyone's card.
func exchangeCards(t *testing.T, rcs ...*apisdk.RoomCrypto) {
	t.Helper()
	for _, from := range rcs {
		ev := cardEvent(t, from)
		for _, to := range rcs {
			if _, handled := handleCryptoEvent(ev, to); !handled {
				t.Fatal("card event not handled")
			}
		}
	}
}

// ownedChatModel is a chat model in a room owned by ownerID.
func ownedChatModel(t *testing.T, ownerID string) (model, string) {
	t.Helper()
	tm, err := NewModel()
	if err != nil {
		t.Fatal(err)
	}
	m := tm.(model)
	m.client = apisdk.NewClient()
	if ownerID == "" {
		ownerID = m.userID
	}
	key := testRoomKey(t)
	m, _ = m.enterChat(&apisdk.RoomResponse{
		ID:          "r",
		JoinCode:    "ABC",
		QRCodeURL:   "http://visper.test/join?joinCode=ABC",
		Owner:       apisdk.UserResponse{ID: ownerID},
		CurrentUser: apisdk.UserResponse{ID: m.userID},
		ExpiresAt:   time.Now().Add(time.Hour),
	}, key)
	t.Cleanup(func() { m.clearChat() })
	return m, key
}

func TestRelabelledOrUnknownSenderShowsUnverified(t *testing.T) {
	m, key := ownedChatModel(t, "")
	me := m.state.chat.crypto
	mallory := newPeer(t, "mallory", "r", m.userID, key)
	exchangeCards(t, me, mallory)

	sealed, err := mallory.SealMessage("send me the codes", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if got := decryptContent(me, sealed, true, sealedAs{from: "mallory", messageID: "m1", at: now}); got.text != "send me the codes" {
		t.Fatalf("genuine sender: got %q", got.text)
	}
	// The server relays Mallory's message as someone else's.
	if got := decryptContent(me, sealed, true, sealedAs{from: m.userID, messageID: "m2", at: now}); got.text != "[Unverified message]" {
		t.Fatalf("relabelled sender: got %q", got.text)
	}
	// ...or as a reaction.
	if got := decryptContent(me, sealed, true, sealedAs{reaction: true, from: "mallory", messageID: "m1"}); got.text != "[Unverified message]" {
		t.Fatalf("message as reaction: got %q", got.text)
	}
	// Plaintext in an encrypted room can only come from the server.
	if got := decryptContent(me, "hi", false, sealedAs{from: "mallory", messageID: "m3"}); got.text != "[Unverified message]" {
		t.Fatalf("plaintext: got %q", got.text)
	}

	// Someone whose card we never saw can't be verified.
	eve := newPeer(t, "eve", "r", m.userID, key)
	fromEve, _ := eve.SealMessage("hi", "")
	if got := decryptContent(me, fromEve, true, sealedAs{from: "eve", messageID: "m4", at: now}); got.text != "[Unverified message]" {
		t.Fatalf("unknown sender: got %q", got.text)
	}

	// A message received through the room's events goes the same way.
	msg, ok := parseRoomEvent(apisdk.WSMessage{Type: apisdk.MessageReceived, Data: map[string]any{
		"id": "m5", "userId": m.userID, "username": "me", "content": sealed, "encrypted": true,
		"timestamp": now.UTC().Format(time.RFC3339),
	}}, func(c string, enc bool, want sealedAs) decrypted { return decryptContent(me, c, enc, want) })
	if !ok || msg.(messageReceivedMsg).message.Content != "[Unverified message]" {
		t.Fatalf("relabelled event = %#v", msg)
	}
}

func TestRekeyEventSwitchesTheInviteToTheNewKey(t *testing.T) {
	m, oldKey := ownedChatModel(t, "alice")
	me := m.state.chat.crypto
	alice := newPeer(t, "alice", "r", "alice", oldKey)
	exchangeCards(t, me, alice)
	oldInvite := m.state.chat.inviteCode()

	rekey, newKey, missing, err := alice.NewRekey([]string{"alice", m.userID}, 0)
	if err != nil || len(missing) != 0 {
		t.Fatalf("NewRekey: missing %v, %v", missing, err)
	}
	// The event arrives as decoded JSON.
	raw, _ := json.Marshal(rekey)
	var data map[string]any
	_ = json.Unmarshal(raw, &data)

	got, handled := handleCryptoEvent(apisdk.WSMessage{Type: apisdk.RoomRekeyed, Data: data}, me)
	if !handled {
		t.Fatal("rekey event not handled")
	}
	if _, ok := got.(rekeyAppliedMsg); !ok {
		t.Fatalf("rekey event = %#v, want rekeyAppliedMsg", got)
	}
	m, _ = m.rekeyAppliedUpdate()

	urlKey, _ := apisdk.RoomKeyToURL(newKey)
	if invite := m.state.chat.inviteCode(); invite == oldInvite || invite != "ABC."+urlKey {
		t.Fatalf("invite = %q, want the new key", invite)
	}
	if !strings.HasSuffix(m.state.chat.room.QRCodeURL, "#key="+urlKey) {
		t.Fatalf("QR URL = %q, want the new key in its fragment", m.state.chat.room.QRCodeURL)
	}
	if !strings.Contains(fmt.Sprintf("%+v", m.state.toast), "rotated") {
		t.Fatalf("toast = %+v, want the key rotation notice", m.state.toast)
	}

	// A rekey not signed by the owner is refused.
	mallory := newPeer(t, "mallory", "r", "mallory", newKey)
	exchangeCards(t, me, mallory)
	forged, _, _, _ := mallory.NewRekey([]string{"mallory", m.userID}, 5)
	raw, _ = json.Marshal(forged)
	data = nil
	_ = json.Unmarshal(raw, &data)
	got, _ = handleCryptoEvent(apisdk.WSMessage{Type: apisdk.RoomRekeyed, Data: data}, me)
	if _, ok := got.(rekeyRejectedMsg); !ok || me.CurrentKey() != newKey {
		t.Fatalf("forged rekey = %#v, key changed: %v", got, me.CurrentKey() != newKey)
	}
}

func TestOwnerRotatesTheKeyWhenSomeoneLeaves(t *testing.T) {
	m, _ := ownedChatModel(t, "")
	m.state.chat.members.add(apisdk.UserResponse{ID: "bob", Username: "bob"})

	m, cmd, _ := m.chatEventUpdate(memberLeftMsg{userID: "bob", username: "bob"})
	if !m.state.chat.rekeying || cmd == nil {
		t.Fatal("the owner did not start a key rotation")
	}
	// A second departure while it runs is queued, not raced.
	m, _, _ = m.chatEventUpdate(kickedMsg{userID: "carol", username: "carol"})
	if !m.state.chat.rekeyPending {
		t.Fatal("second departure was not queued")
	}
	m, _ = m.keyRotatedUpdate(keyRotatedMsg{session: m.state.chat.session, err: fmt.Errorf("offline")})
	if !m.state.chat.rekeying || m.state.chat.rekeyPending {
		t.Fatal("the queued rotation did not start")
	}
}

func TestMembersDoNotRotateTheKey(t *testing.T) {
	m, _ := ownedChatModel(t, "alice")
	m, _, _ = m.chatEventUpdate(memberLeftMsg{userID: "bob", username: "bob"})
	if m.state.chat.rekeying {
		t.Fatal("a member who doesn't own the room started a rotation")
	}
}

func TestAIRewriteAsksBeforeSendingTheDraft(t *testing.T) {
	m := newChatModel(t)
	m.state.chat.messageInput.SetValue("hello there")

	m, cmd := m.rewriteDraft()
	if cmd != nil || m.state.chat.aiEnhancing {
		t.Fatal("the draft was sent without asking")
	}
	if !m.state.dialog.open || m.state.dialog.kind != dialogAIConsent || !strings.Contains(m.state.dialog.content, "WITHOUT end-to-end encryption") {
		t.Fatalf("dialog = %+v, want the AI consent dialog", m.state.dialog)
	}

	// Declining sends nothing.
	declined, cmd := m.chatDialogUpdate(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd != nil || declined.aiConsent || declined.state.chat.aiEnhancing {
		t.Fatal("declining still sent the draft")
	}

	m, cmd = m.chatDialogUpdate(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !m.aiConsent || !m.state.chat.aiEnhancing || cmd == nil {
		t.Fatal("agreeing did not send the draft")
	}

	// Asked once per session.
	m.state.chat.aiEnhancing = false
	m, cmd = m.rewriteDraft()
	if m.state.dialog.open || cmd == nil {
		t.Fatal("asked again after agreeing")
	}
}

func TestSwappedImageIsShownAsMismatched(t *testing.T) {
	saved := resource.Resource.Api.Url
	t.Cleanup(func() { resource.Resource.Api.Url = saved })
	resource.Resource.Api.Url = "https://x:5004"

	m := newChatModel(t)
	msg := apisdk.MessageResponse{ID: "m1", UserID: "u", Username: "u", Content: "https://x/d/r/f.enc#cat.png"}
	m = withMessages(m, msg)
	m = m.imageFetchedUpdate(imageFetchedMsg{messageID: "m1", err: fmt.Errorf("download: %w", apisdk.ErrFileMismatch)})
	if !m.state.chat.images.mismatched["m1"] {
		t.Fatal("mismatch not recorded")
	}
	m.state.chat.images.failed["m1"] = true
	if got := m.messageContentView(msg, 60); !strings.Contains(got, "doesn't match its message") {
		t.Fatalf("view = %q", got)
	}
}
