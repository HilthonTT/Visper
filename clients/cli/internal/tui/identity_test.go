package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/settings"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func TestMain(m *testing.M) {
	// Tests keep identities in memory, never in the user's config dir.
	identityPath = func() string { return "" }
	os.Exit(m.Run())
}

// withIdentityFile points NewModel at a temporary identity file.
func withIdentityFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), settings.IdentityFile)
	identityPath = func() string { return path }
	t.Cleanup(func() { identityPath = func() string { return "" } })
	return path
}

func newModelT(t *testing.T) model {
	t.Helper()
	tm, err := NewModel()
	if err != nil {
		t.Fatal(err)
	}
	m := tm.(model)
	m.client = apisdk.NewClient()
	return m
}

func TestIdentityIsKeptAcrossRuns(t *testing.T) {
	withIdentityFile(t)
	first := newModelT(t)
	if first.identityStore == nil {
		t.Fatal("identity not stored")
	}
	second := newModelT(t)
	if second.userID != first.userID || second.userToken != first.userToken {
		t.Fatal("a second run got a new user")
	}
	if second.deviceCode() != first.deviceCode() {
		t.Fatal("the device code changed across runs")
	}
	if second.identity.SigningPublicKey() != first.identity.SigningPublicKey() {
		t.Fatal("the signing key changed across runs")
	}
}

func TestEphemeralIdentityWithoutFile(t *testing.T) {
	a, b := newModelT(t), newModelT(t)
	if a.identityStore != nil || a.userID == b.userID {
		t.Fatal("identities without a file must be per run")
	}
}

func TestCorruptIdentityFileStartsFreshAndKeepsBackup(t *testing.T) {
	path := withIdentityFile(t)
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := newModelT(t)
	if m.identityStore == nil || m.userID == "" {
		t.Fatal("no fresh identity after a corrupt file")
	}
	if b, err := os.ReadFile(path + ".bak"); err != nil || string(b) != "garbage" {
		t.Fatalf("backup = %q, %v", b, err)
	}
}

func TestRoomIsRememberedAndReopenedWithItsKeys(t *testing.T) {
	withIdentityFile(t)
	m := newModelT(t)
	key := testRoomKey(t)
	room := &apisdk.RoomResponse{
		ID: "r", JoinCode: "ABC",
		Owner:       apisdk.UserResponse{ID: "owner", Username: "Olive"},
		CurrentUser: apisdk.UserResponse{ID: m.userID},
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	m, _ = m.enterChat(room, key)

	// A rekey and a verified member, as they'd happen in the room.
	rc := m.state.chat.crypto
	owner := newPeer(t, "owner", "r", "owner", key)
	exchangeCards(t, rc, owner)
	if err := rc.VerifyMember("owner", owner.MyFingerprint()); err != nil {
		t.Fatal(err)
	}
	m, _ = m.verifyMember("owner", false)
	rekey, newKey, _, err := owner.NewRekey([]string{"owner", m.userID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := rc.ApplyRekey(rekey); err != nil || !changed {
		t.Fatalf("ApplyRekey = %v, %v", changed, err)
	}
	m, _ = m.rekeyAppliedUpdate()
	m.clearChat()

	// Next run: the room is listed, and reopening restores keys and pins.
	next := newModelT(t)
	rooms := next.rememberedRooms()
	if len(rooms) != 1 || rooms[0].ID != "r" || rooms[0].OwnerID != "owner" {
		t.Fatalf("remembered rooms = %+v", rooms)
	}
	tm, _ := next.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next = tm.(model)
	next, _ = next.gotoJoinRoom()
	if !strings.Contains(next.joinRoomView(30), "#ABC") {
		t.Fatal("join page doesn't list the remembered room")
	}
	next, _ = next.enterReopenedRoom(roomReopenedMsg{room: room, stored: rooms[0]})
	t.Cleanup(func() { next.clearChat() })
	got := next.state.chat.crypto
	if got == nil || got.CurrentKey() != newKey || got.Epoch() != 1 {
		t.Fatal("reopened room lost its rotated key")
	}
	_ = got.AddCard("owner", mustCardT(t, owner))
	if got.KeyStatus("owner") != apisdk.KeyVerified {
		t.Fatal("verified owner not pinned after reopening")
	}
}

func TestReopenRefusesAnotherOwner(t *testing.T) {
	withIdentityFile(t)
	m := newModelT(t)
	stored := settings.StoredRoom{ID: "r", OwnerID: "owner", Keys: []string{testRoomKey(t)}}
	if err := m.identityStore.SaveRoom(stored); err != nil {
		t.Fatal(err)
	}
	room := &apisdk.RoomResponse{ID: "r", Owner: apisdk.UserResponse{ID: "mallory"}, CurrentUser: apisdk.UserResponse{ID: m.userID}}
	m, cmd := m.enterReopenedRoom(roomReopenedMsg{room: room, stored: stored})
	if m.state.chat.room != nil {
		t.Fatal("entered a room whose owner changed")
	}
	if _, ok := cmd().(visibleError); !ok {
		t.Fatal("no error shown")
	}
	if _, ok := m.identityStore.Room("r"); ok {
		t.Fatal("room with a changed owner still remembered")
	}
}

func TestLeavingForgetsTheRoom(t *testing.T) {
	withIdentityFile(t)
	m := newModelT(t)
	m, _ = m.enterChat(&apisdk.RoomResponse{ID: "r", JoinCode: "ABC", Owner: apisdk.UserResponse{ID: "o"}, ExpiresAt: time.Now().Add(time.Hour)}, testRoomKey(t))
	if _, ok := m.identityStore.Room("r"); !ok {
		t.Fatal("room not remembered on entering")
	}
	m, _ = m.leaveChat()
	if _, ok := m.identityStore.Room("r"); ok {
		t.Fatal("room still remembered after leaving")
	}
}

func TestReopenOfAGoneRoomForgetsIt(t *testing.T) {
	withIdentityFile(t)
	m := newModelT(t)
	_ = m.identityStore.SaveRoom(settings.StoredRoom{ID: "r", JoinCode: "ABC", OwnerID: "o", Keys: []string{testRoomKey(t)}})
	m, _ = m.gotoJoinRoom()
	tm, _ := m.Update(roomReopenFailedMsg{roomID: "r", gone: true})
	m = tm.(model)
	if _, ok := m.identityStore.Room("r"); ok || len(m.state.joinRoom.rooms) != 0 {
		t.Fatal("gone room still remembered")
	}
	if m.state.joinRoom.error == "" {
		t.Fatal("no message about the gone room")
	}
}

func mustCardT(t *testing.T, rc *apisdk.RoomCrypto) string {
	t.Helper()
	card, err := rc.MyCard()
	if err != nil {
		t.Fatal(err)
	}
	return card
}
