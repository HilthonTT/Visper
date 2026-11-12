package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func TestParseNotificationRequiresSealedKey(t *testing.T) {
	data := map[string]any{"room_id": "r", "join_code": "ABC123", "secure_code": "s"}
	if _, ok := parseNotification(apisdk.NotificationWSMessage{Type: apisdk.NotificationRoomInvite, Data: data}); ok {
		t.Fatal("accepted an invite without a sealed key")
	}
	data["sealed_key"] = "sealed"
	msg, ok := parseNotification(apisdk.NotificationWSMessage{Type: apisdk.NotificationRoomInvite, Data: data})
	if !ok || msg.(roomInviteMsg).sealedKey != "sealed" {
		t.Fatalf("parseNotification = %+v, %v", msg, ok)
	}
}

// An invite sealed to another device (or an older run of this one) must not
// join the room.
func TestJoinFromInviteRejectsKeySealedToAnotherDevice(t *testing.T) {
	tm, err := NewModel()
	if err != nil {
		t.Fatal(err)
	}
	m := tm.(model)
	m.client = apisdk.NewClient()

	other, _ := apisdk.NewDeviceKey()
	key, _ := apisdk.NewRoomKey()
	sealed, err := apisdk.SealRoomKey(key, other.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	msg := m.joinFromInvite(roomInviteMsg{roomID: "r", joinCode: "ABC123", secureCode: "s", sealedKey: sealed})()
	if _, ok := msg.(visibleError); !ok {
		t.Fatalf("joinFromInvite = %#v, want a visibleError", msg)
	}
}

func TestJoinPageNeedsTheRoomKey(t *testing.T) {
	tm, err := NewModel()
	if err != nil {
		t.Fatal(err)
	}
	m := tm.(model)
	m, _ = m.gotoJoinRoom()

	for _, code := range []string{"ABC123", "ABC123.not-a-key"} {
		m.state.joinRoom.input.SetValue(code)
		mm, cmd := m.joinRoom()
		if cmd != nil || mm.state.joinRoom.joining || mm.state.joinRoom.error == "" {
			t.Fatalf("%q: joined without a valid key", code)
		}
	}

	key, _ := apisdk.NewRoomKey()
	code, _ := apisdk.FormatInviteCode("ABC123", key)
	m.state.joinRoom.input.SetValue(code)
	if mm, cmd := m.joinRoom(); cmd == nil || !mm.state.joinRoom.joining {
		t.Fatal("a full invite code was refused")
	}
}

func TestQRDialogCopiesInviteCode(t *testing.T) {
	m := newChatModel(t)
	m = m.openQRCodeDialog()
	if !strings.Contains(m.state.dialog.content, ".") {
		t.Fatalf("QR dialog shows %q, want the invite code", m.state.dialog.content)
	}
	m, cmd := m.chatDialogUpdate(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.state.dialog.open || cmd == nil {
		t.Fatal("c did not copy the invite code")
	}
}
