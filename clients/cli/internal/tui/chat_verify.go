package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The server relays key cards, so it could slip in one in a member's name
// (with the help of someone who has the room key) and get the next room key
// sealed to it. Members rule that out by reading their key fingerprints to
// each other over a call or in person. Verifying a member pins their card
// (see apisdk.RoomCrypto.VerifyMember): a different one is then refused, and
// shows as a changed key.

// keyChangedMsg reports that a member we verified published a different key
// card.
type keyChangedMsg struct{ userID string }

// openVerifyKeyDialog shows our fingerprint and the highlighted member's, to
// compare and mark them verified (or unverified).
func (m model) openVerifyKeyDialog() (model, tea.Cmd) {
	selected, ok := m.state.chat.members.highlighted()
	rc := m.state.chat.crypto
	if !ok || rc == nil {
		return m, nil
	}
	mine := rc.MyFingerprint()
	if m.isCurrentUser(selected.ID) {
		return m.alert(theme.ToneInfo, "Your key",
			mine+"\n\nRead this out when someone verifies you. It changes every time you start visper."), nil
	}

	theirs := rc.Fingerprint(selected.ID)
	if theirs == "" {
		return m.alert(theme.ToneWarning, "No key for "+selected.Username,
			selected.Username+" hasn't published a key yet, so there is nothing to verify."), nil
	}

	name := selected.Username
	content := fmt.Sprintf("Your key\n%s\n\n%s's key\n%s\n\n", mine, name, theirs)
	d := dialogState{kind: dialogVerifyKey, tone: theme.ToneInfo, title: "Verify " + name + "?"}
	switch rc.KeyStatus(selected.ID) {
	case apisdk.KeyVerified:
		d.kind, d.tone, d.title = dialogUnverifyKey, theme.ToneSuccess, name+" is verified"
		content += "You compared this key with " + name + ". Only it is trusted for them."
	case apisdk.KeyChanged:
		d.tone, d.title = theme.ToneWarning, name+"'s key changed"
		content += name + "'s key changed since you verified it. Their client may have restarted, " +
			"or someone may be posing as them. Until you verify the new key, only the old one is trusted."
	default:
		content += "Ask " + name + " to read out their key, over a call or in person. " +
			"Verify only if it matches the digits above."
	}
	d.content = content
	m.state.chat.members.keysUserID = selected.ID
	return m.openDialog(d), nil
}

// verifyMember pins (or, with unverify, unpins) a member's key.
func (m model) verifyMember(userID string, unverify bool) (model, tea.Cmd) {
	rc := m.state.chat.crypto
	if rc == nil || userID == "" {
		return m, nil
	}
	name := m.state.chat.members.name(userID)
	if name == "" {
		name = "That member"
	}
	if unverify {
		rc.UnverifyMember(userID)
		return m.toast(theme.ToneInfo, name+" is no longer verified.")
	}
	// The fingerprint shown in the dialog: verify exactly what was compared.
	if err := rc.VerifyMember(userID, rc.Fingerprint(userID)); err != nil {
		return m.toast(theme.ToneDanger, fmt.Sprintf("Couldn't verify %s: %v", name, err))
	}
	// Toasts are one line: keep these short enough to show in full.
	return m.toast(theme.ToneSuccess, name+" verified: only this key is trusted.")
}

func (m model) keyChangedUpdate(msg keyChangedMsg) (model, tea.Cmd) {
	name := m.state.chat.members.name(msg.userID)
	if name == "" {
		name = "A member you verified"
	}
	m, cmd := m.toast(theme.ToneWarning, name+" has a new key: check with ctrl+s, ctrl+k.")
	return m, tea.Batch(cmd, m.waitForRoomEvent())
}
