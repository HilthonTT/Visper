package tui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
)

// Everything we send is signed with our identity's key and sealed with the
// room key; readers check the signature against the sender's key card (see
// apisdk.RoomCrypto). On entering a room we load the members' cards and
// publish ours; afterwards cards and rekeys arrive over the WebSocket.
//
// When a member leaves or is kicked, the owner's client rotates the room key
// so they can't read what is sent afterwards.

const keyRotatedText = "Room key rotated. Old invites no longer work; ctrl+p shows the new one."

type (
	// cryptoSyncedMsg reports that the room's key cards were loaded and
	// ours published (or why not), on entering the room.
	cryptoSyncedMsg struct {
		session     context.Context
		rekeyNeeded bool
		err         error
	}
	// rekeyAppliedMsg reports that a rekey from the owner switched us to a
	// new room key.
	rekeyAppliedMsg struct{}
	// rekeyRejectedMsg reports a rekey that was not signed by the owner.
	rekeyRejectedMsg struct{ err error }
	// rekeyExcludedMsg reports a rekey from the owner, made while we were
	// in the room, that has no key for us.
	rekeyExcludedMsg struct{}
	// keyRotatedMsg is the result of rotating the room key, as its owner.
	keyRotatedMsg struct {
		session context.Context
		missing []string
		err     error
	}
)

// syncRoomCrypto loads the room's key cards and latest rekey, then
// publishes our card for the current key.
func (m model) syncRoomCrypto() tea.Cmd {
	rc, room := m.state.chat.crypto, m.state.chat.room
	if rc == nil || room == nil {
		return nil
	}
	session, client, opts, roomID := m.state.chat.session, m.client, m.requestOpts(), room.ID
	return func() tea.Msg {
		state, changed, err := client.Room.SyncCrypto(session, roomID, rc, opts...)
		// SyncCrypto already published a card if the key changed.
		if err == nil && !changed {
			err = client.Room.PublishKeyCard(session, roomID, rc, opts...)
		}
		return cryptoSyncedMsg{session: session, rekeyNeeded: state != nil && state.RekeyNeeded, err: err}
	}
}

func (m model) cryptoSyncedUpdate(msg cryptoSyncedMsg) (model, tea.Cmd) {
	if msg.session != m.state.chat.session || m.state.chat.room == nil {
		return m, nil // we already left this room
	}
	var toastCmd, rotateCmd tea.Cmd
	if msg.err != nil {
		log.Printf("Failed to sync room keys: %v", msg.err)
		m, toastCmd = m.toast(theme.ToneWarning, "Couldn't load the members' keys, so messages may show as unverified.")
	}
	// Syncing may have applied a rekey we missed.
	m.rememberRoom()
	if msg.rekeyNeeded && m.state.chat.isRoomOwner {
		// Someone left while no owner's client was around to rotate.
		m, rotateCmd = m.rotateRoomKey()
	}
	// Connect only now, so the history the WebSocket replays is verified
	// against the cards we just loaded.
	return m, tea.Batch(toastCmd, rotateCmd, m.connectRoom(m.state.chat.room.ID))
}

// handleCryptoEvent applies key cards and rekeys to rc as they arrive; it
// runs on the WebSocket's goroutine, before later events are decrypted.
// handled reports whether wsMsg was such an event; msg, if not nil, is what
// to tell Update.
func handleCryptoEvent(wsMsg apisdk.WSMessage, rc *apisdk.RoomCrypto) (msg tea.Msg, handled bool) {
	if wsMsg.Type != apisdk.MemberCard && wsMsg.Type != apisdk.RoomRekeyed {
		return nil, false
	}
	data, ok := wsMsg.Data.(map[string]any)
	if !ok || rc == nil {
		return nil, true
	}

	if wsMsg.Type == apisdk.MemberCard {
		userID, okID := stringField(data, "userId")
		card, okCard := stringField(data, "card")
		if okID && okCard {
			before := rc.KeyStatus(userID)
			if err := rc.AddCard(userID, card); err != nil {
				log.Printf("Ignoring key card of %s: %v", userID, err)
			}
			if before == apisdk.KeyVerified && rc.KeyStatus(userID) == apisdk.KeyChanged {
				return keyChangedMsg{userID: userID}, true
			}
		}
		return nil, true
	}

	// This rekey arrived live, so we were a member when it was made: one
	// without a key for us left us out (unlike a rekey found when syncing on
	// entry, which may simply predate our invite).
	result, err := rc.ApplyRekeyResult(parseRekey(data))
	switch {
	case err != nil:
		log.Printf("Rejected rekey: %v", err)
		return rekeyRejectedMsg{err: err}, true
	case result == apisdk.RekeyApplied:
		return rekeyAppliedMsg{}, true
	case result == apisdk.RekeyExcluded:
		return rekeyExcludedMsg{}, true
	}
	return nil, true
}

// parseRekey reads a RoomRekeyed payload.
func parseRekey(data map[string]any) *apisdk.Rekey {
	r := &apisdk.Rekey{Keys: map[string]string{}}
	if epoch, ok := data["epoch"].(float64); ok {
		r.Epoch = int(epoch)
	}
	r.Signature, _ = data["signature"].(string)
	if keys, ok := data["keys"].(map[string]any); ok {
		for id, v := range keys {
			if sealed, ok := v.(string); ok {
				r.Keys[id] = sealed
			}
		}
	}
	return r
}

// rekeyAppliedUpdate publishes our card for the new key and points the
// invite at it.
func (m model) rekeyAppliedUpdate() (model, tea.Cmd) {
	m.state.chat.keyExcluded = false
	m = m.refreshInviteKey()
	m.rememberRoom()
	rc := m.state.chat.crypto
	publish := m.roomRequest("publish your key card", func(ctx context.Context, c *apisdk.Client, roomID string, opts []option.RequestOption) error {
		return c.Room.PublishKeyCard(ctx, roomID, rc, opts...)
	})
	m, toastCmd := m.toast(theme.ToneInfo, keyRotatedText)
	return m, tea.Batch(publish, toastCmd, m.waitForRoomEvent())
}

// keyExcludedText explains why a member left out of a rotation can't send.
const keyExcludedText = "The owner changed the room key without you (they had no key card for you), so anything you send could be read by members who left. Sending is off until you're given the new key: ask the owner for a new invite."

// rekeyExcludedUpdate stops sending after a rotation that left us out. Our
// key card is published again so the owner's next rotation can include us.
func (m model) rekeyExcludedUpdate() (model, tea.Cmd) {
	m.state.chat.keyExcluded = true
	rc := m.state.chat.crypto
	publish := m.roomRequest("publish your key card", func(ctx context.Context, c *apisdk.Client, roomID string, opts []option.RequestOption) error {
		return c.Room.PublishKeyCard(ctx, roomID, rc, opts...)
	})
	m = m.alert(theme.ToneDanger, "Left out of the new room key", keyExcludedText)
	return m, tea.Batch(publish, m.waitForRoomEvent())
}

// sendingBlocked returns a command showing why nothing can be sent, or nil
// if sending is allowed.
func (m model) sendingBlocked() tea.Cmd {
	if !m.state.chat.keyExcluded {
		return nil
	}
	return func() tea.Msg { return visibleError{message: keyExcludedText} }
}

// refreshInviteKey puts the current room key in the invite link.
func (m model) refreshInviteKey() model {
	if key := m.state.chat.roomKey(); key != "" {
		m.state.chat.room = withInviteKey(m.state.chat.room, key)
	}
	return m
}

// memberDeparted records that someone else left or was kicked and, if we
// own the room, rotates its key so they can't read what comes next.
func (m model) memberDeparted(userID string) (model, tea.Cmd) {
	rc := m.state.chat.crypto
	if rc == nil || m.isCurrentUser(userID) || !m.state.chat.isRoomOwner {
		return m, nil
	}
	rc.MarkDeparted(userID)
	return m.rotateRoomKey()
}

// rotateRoomKey makes a new room key for the remaining members, as the
// owner. Rotations never overlap: one asked for while another runs follows it.
func (m model) rotateRoomKey() (model, tea.Cmd) {
	c := &m.state.chat
	if c.crypto == nil || c.room == nil || !c.crypto.IsOwner() {
		return m, nil
	}
	if c.rekeying {
		c.rekeyPending = true
		return m, nil
	}
	c.rekeying = true

	rc, session, client, opts, roomID := c.crypto, c.session, m.client, m.requestOpts(), c.room.ID
	return m, func() tea.Msg {
		missing, err := client.Room.RotateKey(session, roomID, rc, opts...)
		if isConflict(err) {
			// Our epoch was out of date: catch up and try once more.
			if _, _, syncErr := client.Room.SyncCrypto(session, roomID, rc, opts...); syncErr == nil {
				missing, err = client.Room.RotateKey(session, roomID, rc, opts...)
			}
		}
		return keyRotatedMsg{session: session, missing: missing, err: err}
	}
}

func (m model) keyRotatedUpdate(msg keyRotatedMsg) (model, tea.Cmd) {
	if msg.session != m.state.chat.session {
		return m, nil // we already left this room
	}
	c := &m.state.chat
	c.rekeying = false
	var next tea.Cmd
	if c.rekeyPending {
		c.rekeyPending = false
		m, next = m.rotateRoomKey()
	}
	if msg.err != nil {
		log.Printf("Failed to rotate the room key: %v", msg.err)
		m, toastCmd := m.toast(theme.ToneDanger, fmt.Sprintf("Couldn't rotate the room key: %v", msg.err))
		return m, tea.Batch(toastCmd, next)
	}

	m = m.refreshInviteKey()
	m.rememberRoom()
	text := keyRotatedText
	if n := len(msg.missing); n > 0 {
		names := make([]string, 0, n)
		for _, id := range msg.missing {
			name := m.state.chat.members.name(id)
			if name == "" {
				name = "a member"
			}
			names = append(names, name)
		}
		text = fmt.Sprintf("Room key rotated, but %s can't get it (no key card): send them a new invite.", strings.Join(names, ", "))
	}
	m, toastCmd := m.toast(theme.ToneInfo, text)
	return m, tea.Batch(toastCmd, next)
}

// isConflict reports whether err is the API's 409 (out-of-date rekey).
func isConflict(err error) bool {
	if apiErr, ok := errors.AsType[*apisdk.Error](err); ok {
		return apiErr.StatusCode == http.StatusConflict
	}
	return false
}
