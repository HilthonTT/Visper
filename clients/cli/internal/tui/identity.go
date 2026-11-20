package tui

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/settings"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The CLI keeps its identity (user token, signing key, device key) and the
// rooms it is in (their keys, pinned owner and verified members) in an
// owner-only file, settings.IdentityPath. That lets it come back to a room
// after a restart as the same member, with the same key fingerprint, so:
//
//   - an owner who comes back rotates the key after members left meanwhile
//     (the API's rekey_needed, see cryptoSyncedUpdate);
//   - verified members stay verified;
//   - the device code the web client sends invites to stays the same.
//
// Quitting therefore keeps us in the room (only ctrl+b leaves it); the join
// page lists the rooms to reopen. `visper -forget` deletes the file.

// identityPath returns where the identity is kept; "" keeps it in memory
// only, as before (tests use that).
var identityPath = settings.IdentityPath

// loadIdentity returns the stored identity, or a new one (saved for next
// time). store is nil when the identity can't be kept: it then lasts only
// for this run.
func loadIdentity(path string) (store *settings.IdentityStore, userToken string, identity *apisdk.Identity, err error) {
	if path != "" {
		var openErr error
		store, openErr = settings.OpenIdentityStore(path)
		if openErr != nil {
			log.Printf("Identity: %v", openErr)
		}
		if token, sign, dev, ok := store.Identity(); ok {
			if identity, err := restoreIdentity(token, sign, dev); err == nil {
				if err := store.RemoveExpired(time.Now()); err != nil {
					log.Printf("Identity: couldn't drop expired rooms: %v", err)
				}
				return store, token, identity, nil
			} else {
				// Parsed but unusable: keep the file for inspection rather
				// than overwrite it.
				log.Printf("Identity: stored keys are invalid (%v); moving the file aside", err)
				if err := store.MoveAside(); err != nil {
					log.Printf("Identity: %v; keeping this identity in memory only", err)
					store = nil
				}
			}
		}
	}

	userToken = apisdk.NewUserToken()
	device, err := apisdk.NewDeviceKey()
	if err != nil {
		return nil, "", nil, err
	}
	identity, err = apisdk.NewIdentity(apisdk.UserIDFromToken(userToken), device)
	if err != nil {
		return nil, "", nil, err
	}
	if store != nil {
		if err := store.SetIdentity(userToken, identity.ExportSigningKey(), device.Export()); err != nil {
			log.Printf("Identity: %v; keeping this identity in memory only", err)
			store = nil
		}
	}
	return store, userToken, identity, nil
}

func restoreIdentity(token, signingKey, deviceKey string) (*apisdk.Identity, error) {
	device, err := apisdk.RestoreDeviceKey(deviceKey)
	if err != nil {
		return nil, err
	}
	return apisdk.RestoreIdentity(apisdk.UserIDFromToken(token), signingKey, device)
}

// rememberRoom saves the current room's keys, pinned owner and verified
// members, so it can be reopened after a restart.
func (m model) rememberRoom() {
	c := m.state.chat
	if m.identityStore == nil || c.room == nil || c.crypto == nil {
		return
	}
	st := c.crypto.State()
	err := m.identityStore.SaveRoom(settings.StoredRoom{
		ID:        c.room.ID,
		JoinCode:  c.roomCode,
		OwnerID:   c.crypto.OwnerID(),
		OwnerName: c.room.Owner.Username,
		ExpiresAt: c.room.ExpiresAt,
		Keys:      st.Keys,
		Epoch:     st.Epoch,
		Verified:  st.Verified,
		SavedAt:   time.Now(),
	})
	if err != nil {
		log.Printf("Couldn't remember room %s: %v", c.room.ID, err)
	}
}

// storedRoom returns the remembered state of roomID.
func (m model) storedRoom(roomID string) (settings.StoredRoom, bool) {
	if m.identityStore == nil {
		return settings.StoredRoom{}, false
	}
	return m.identityStore.Room(roomID)
}

// forgetRoom drops a room we are no longer in.
func (m model) forgetRoom(roomID string) {
	if m.identityStore == nil || roomID == "" {
		return
	}
	if err := m.identityStore.RemoveRoom(roomID); err != nil {
		log.Printf("Couldn't forget room %s: %v", roomID, err)
	}
}

// forgetCurrentRoom is forgetRoom for the room on screen.
func (m model) forgetCurrentRoom() {
	if m.state.chat.room != nil {
		m.forgetRoom(m.state.chat.room.ID)
	}
}

// rememberedRooms lists the rooms that can be reopened, newest first.
func (m model) rememberedRooms() []settings.StoredRoom {
	if m.identityStore == nil {
		return nil
	}
	var rooms []settings.StoredRoom
	now := time.Now()
	for _, r := range m.identityStore.Rooms() {
		if !r.Expired(now) {
			rooms = append(rooms, r)
		}
	}
	return rooms
}

type (
	// roomReopenedMsg is a remembered room the API still has us in.
	roomReopenedMsg struct {
		room   *apisdk.RoomResponse
		stored settings.StoredRoom
	}
	// roomReopenFailedMsg is a remembered room that couldn't be reopened;
	// gone means the API no longer has it (or us in it).
	roomReopenFailedMsg struct {
		roomID string
		err    error
		gone   bool
	}
)

// reopenRoom asks the API for a remembered room; we are still a member, so
// no join code or invite is needed.
func (m model) reopenRoom(r settings.StoredRoom) tea.Cmd {
	client, ctx, opts := m.client, m.ctx, m.requestOpts()
	return func() tea.Msg {
		reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		room, err := client.Room.Get(reqCtx, r.ID, opts...)
		if err != nil {
			return roomReopenFailedMsg{roomID: r.ID, err: err, gone: isRoomGone(err)}
		}
		return roomReopenedMsg{room: room, stored: r}
	}
}

// isRoomGone reports whether err means the room no longer exists for us.
func isRoomGone(err error) bool {
	if apiErr, ok := errors.AsType[*apisdk.Error](err); ok {
		switch apiErr.StatusCode {
		case http.StatusForbidden, http.StatusNotFound, http.StatusGone:
			return true
		}
	}
	return false
}

// enterReopenedRoom enters a remembered room with its stored keys and the
// owner pinned when we first entered it.
func (m model) enterReopenedRoom(msg roomReopenedMsg) (model, tea.Cmd) {
	r := msg.stored
	if msg.room.Owner.ID != r.OwnerID {
		// The owner can't change: the server is lying, or the room ID now
		// names another room. Don't trust it with our keys.
		m.forgetRoom(r.ID)
		return m, func() tea.Msg {
			return visibleError{message: "That room's owner isn't the one you joined. It was forgotten; ask for a new invite if you still want in."}
		}
	}
	rc, err := apisdk.RestoreRoomCrypto(m.identity, msg.room.ID, r.OwnerID, apisdk.RoomCryptoState{
		Keys: r.Keys, Epoch: r.Epoch, Verified: r.Verified,
	})
	if err != nil {
		m.forgetRoom(r.ID)
		return m, func() tea.Msg {
			return visibleError{message: "The saved keys for that room are damaged. Ask for a new invite."}
		}
	}
	return m.enterRoom(msg.room, rc)
}
