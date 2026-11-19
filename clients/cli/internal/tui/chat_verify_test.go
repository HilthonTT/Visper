package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func TestVerifyMemberFromMemberSearch(t *testing.T) {
	m, key := ownedChatModel(t, "")
	me := m.state.chat.crypto
	bob := newPeer(t, "bob", "r", m.userID, key)
	exchangeCards(t, me, bob)
	m.state.chat.members.list = []apisdk.UserResponse{{ID: m.userID, Username: "me"}, {ID: "bob", Username: "bob"}}
	m.state.chat.members.resetFilter()
	m.state.chat.members.searching = true
	m.state.chat.members.cursor = 1

	ctrlK := tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}
	m, _, handled := m.chatKeyUpdate(ctrlK)
	if !handled || !m.state.dialog.open || m.state.dialog.kind != dialogVerifyKey {
		t.Fatalf("ctrl+k opened %+v", m.state.dialog)
	}
	for _, fp := range []string{me.MyFingerprint(), bob.MyFingerprint()} {
		if !strings.Contains(m.state.dialog.content, fp) {
			t.Fatalf("dialog doesn't show %q:\n%s", fp, m.state.dialog.content)
		}
	}

	m, _ = m.chatDialogUpdate(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if me.KeyStatus("bob") != apisdk.KeyVerified {
		t.Fatal("bob was not verified")
	}
	if !strings.Contains(m.membersPanelView(chatMembersWidth, 20), "✓") {
		t.Fatal("the member list doesn't mark bob as verified")
	}

	// A different card for bob is flagged, and the UI hears about it.
	impostor := newPeer(t, "bob", "r", m.userID, key)
	msg, _ := handleCryptoEvent(cardEvent(t, impostor), me)
	if changed, ok := msg.(keyChangedMsg); !ok || changed.userID != "bob" {
		t.Fatalf("handleCryptoEvent = %#v, want keyChangedMsg for bob", msg)
	}
	if !strings.Contains(m.membersPanelView(chatMembersWidth, 20), "!") {
		t.Fatal("the member list doesn't flag bob's changed key")
	}

	// Opening the dialog again offers to verify the new key; unverify via
	// a verified member's dialog.
	m, _, _ = m.chatKeyUpdate(ctrlK)
	if m.state.dialog.kind != dialogVerifyKey || !strings.Contains(m.state.dialog.content, impostor.MyFingerprint()) {
		t.Fatalf("changed-key dialog = %+v", m.state.dialog)
	}
	m, _ = m.chatDialogUpdate(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if me.KeyStatus("bob") != apisdk.KeyChanged || m.state.chat.members.keysUserID != "" {
		t.Fatal("cancelling changed the key status")
	}
}

func TestOwnKeyShownAsAlert(t *testing.T) {
	m, _ := ownedChatModel(t, "")
	m.state.chat.members.list = []apisdk.UserResponse{{ID: m.userID, Username: "me"}}
	m.state.chat.members.resetFilter()
	m.state.chat.members.searching = true

	m, _, _ = m.chatKeyUpdate(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if m.state.dialog.kind != dialogAlert || !strings.Contains(m.state.dialog.content, m.state.chat.crypto.MyFingerprint()) {
		t.Fatalf("own key dialog = %+v", m.state.dialog)
	}
}
