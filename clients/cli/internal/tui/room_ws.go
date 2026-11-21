package tui

import (
	"context"
	"errors"
	"log"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/strfn"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The room WebSocket delivers the room's live events. Its events are read
// from a channel and wrapped in roomEventMsg, so Update can drop events from a
// room the user already left.

type (
	roomConnectedMsg struct {
		conn    *apisdk.RoomWebSocket
		session context.Context
	}
	roomConnectFailedMsg struct {
		session context.Context
		err     error
	}
	roomListeningMsg struct {
		events  chan tea.Msg
		session context.Context
	}

	// roomEventMsg tags an event read from a room's channel with that
	// channel; otherwise a buffered event or the close of the old channel is
	// applied to the next room (e.g. a "Disconnected" dialog or a foreign
	// message).
	roomEventMsg struct {
		ch  chan tea.Msg
		msg tea.Msg
	}
)

// Room events.
type (
	messageReceivedMsg struct {
		message apisdk.MessageResponse
	}
	// messageUpdatedMsg carries the edit still encrypted: the event does not
	// name the sender, so it is checked against the message it edits.
	messageUpdatedMsg struct {
		messageID string
		content   string
		encrypted bool
		// at is when the server says the edit was made (zero if unknown).
		at time.Time
	}
	messageDeletedMsg struct {
		messageID string
	}
	memberJoinedMsg struct {
		member apisdk.UserResponse
	}
	memberLeftMsg struct {
		userID   string
		username string
	}
	memberListMsg struct {
		members []apisdk.UserResponse
	}
	kickedMsg struct {
		userID   string
		username string
		reason   string
	}
	roomUpdatedMsg struct {
		joinCode string
	}
	roomDeletedMsg      struct{}
	roomDisconnectedMsg struct{}
	roomErrorMsg        struct {
		code    string
		message string
	}
)

func (m model) connectRoom(roomID string) tea.Cmd {
	session, client, opts := m.state.chat.session, m.client, m.requestOpts()
	return func() tea.Msg {
		ws, err := client.Room.ConnectWebSocket(session, roomID, opts...)
		if err != nil {
			log.Printf("Failed to connect WebSocket: %v", err)
			return roomConnectFailedMsg{session: session, err: err}
		}
		return roomConnectedMsg{conn: ws, session: session}
	}
}

// listenRoom starts reading conn and returns the channel its events are
// delivered on, as a roomListeningMsg. rc decrypts and verifies the content
// of an encrypted room; key cards and rekeys are applied to it here, in the
// order they arrive, so the messages after them are read with them.
func listenRoom(session context.Context, conn *apisdk.RoomWebSocket, rc *apisdk.RoomCrypto) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 100)
		send := func(msg tea.Msg) {
			select {
			case events <- msg:
			case <-session.Done():
			}
		}
		decrypt := func(content string, encrypted bool, want sealedAs) decrypted {
			return decryptContent(rc, content, encrypted, want)
		}

		conn.SetMessageHandler(func(wsMsg apisdk.WSMessage) {
			if session.Err() != nil {
				return
			}
			if msg, handled := handleCryptoEvent(wsMsg, rc); handled {
				if msg != nil {
					send(msg)
				}
				return
			}
			if msg, ok := parseRoomEvent(wsMsg, decrypt); ok {
				send(msg)
			}
		})

		go func() {
			defer close(events)
			if err := conn.Listen(session); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("WebSocket listen error: %v", err)
				send(roomDisconnectedMsg{})
			}
		}()

		return roomListeningMsg{events: events, session: session}
	}
}

// waitForRoomEvent reads the next event; a closed channel reads as a
// disconnect.
func waitForRoomEvent(events chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return roomEventMsg{ch: events, msg: roomDisconnectedMsg{}}
		}
		return roomEventMsg{ch: events, msg: msg}
	}
}

// waitForRoomEvent keeps listening on the room WebSocket, if it is open.
func (m model) waitForRoomEvent() tea.Cmd {
	return waitForRoomEvent(m.state.chat.events)
}

// parseRoomEvent turns a WebSocket message into a room event; ok is false for
// unknown or invalid ones. The server's payload keys vary in case, hence the
// alternatives.
func parseRoomEvent(wsMsg apisdk.WSMessage, decrypt decryptFunc) (tea.Msg, bool) {
	if wsMsg.Type == apisdk.MessageDeleted {
		if payload, ok := wsMsg.Data.(apisdk.MessageDeletedPayload); ok {
			return messageDeletedMsg{messageID: payload.ID}, true
		}
	}
	if wsMsg.Type == apisdk.RoomDeleted {
		return roomDeletedMsg{}, true
	}

	data, ok := wsMsg.Data.(map[string]any)
	if !ok {
		if wsMsg.Type == apisdk.MessageDeleted {
			log.Printf("Unknown message deleted payload type: %T - %+v", wsMsg.Data, wsMsg.Data)
		}
		return nil, false
	}

	switch wsMsg.Type {
	case apisdk.MessageReceived:
		id, okID := stringField(data, "id", "ID")
		userID, okUserID := stringField(data, "userId", "UserID", "user_id")
		username, okUsername := stringField(data, "username", "Username")
		content, okContent := stringField(data, "content", "Content")
		if !okID || !okUserID || !okUsername || !okContent {
			log.Printf("Invalid message received payload: %+v (missing fields)", data)
			return nil, false
		}
		encrypted, _ := data["encrypted"].(bool)
		sentAt := eventTime(data)
		createdAt := sentAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		replyTo, _ := stringField(data, "replyToId")
		var updated time.Time
		if ts, ok := stringField(data, "updatedAt"); ok {
			updated, _ = time.Parse(time.RFC3339, ts)
		}
		opened := decrypt(content, encrypted, sealedAs{from: userID, messageID: id, at: sentAt, updated: updated})
		return messageReceivedMsg{message: apisdk.MessageResponse{
			ID:        id,
			RoomID:    wsMsg.RoomID,
			UserID:    userID,
			Username:  username,
			Content:   opened.text,
			Encrypted: encrypted,
			CreatedAt: createdAt.Local(),
			ReplyToID: replyTo,
			Reactions: parseReactions(data["reactions"], id, decrypt),
			FileHash:  opened.fileHash,
		}}, true

	case apisdk.MessageReactions:
		id, ok := stringField(data, "id")
		if !ok {
			return nil, false
		}
		return messageReactionsMsg{messageID: id, reactions: parseReactions(data["reactions"], id, decrypt)}, true

	case apisdk.PresenceUpdate:
		raw, _ := data["online"].([]any)
		online := make([]string, 0, len(raw))
		for _, r := range raw {
			if id, ok := r.(string); ok && id != "" {
				online = append(online, id)
			}
		}
		return presenceMsg{online: online}, true

	case apisdk.MemberTyping:
		userID, okID := stringField(data, "userId")
		username, okName := stringField(data, "username")
		if !okID || !okName {
			return nil, false
		}
		return memberTypingMsg{userID: userID, username: username}, true

	case apisdk.ReadUpdate:
		userID, okID := stringField(data, "userId")
		messageID, okMsg := stringField(data, "messageId")
		if !okID || !okMsg {
			return nil, false
		}
		return readUpdateMsg{userID: userID, messageID: messageID}, true

	case apisdk.MessageUpdated:
		id, okID := stringField(data, "id", "ID")
		content, okContent := stringField(data, "content", "Content")
		if !okID || !okContent {
			log.Printf("Invalid message updated payload: %+v", data)
			return nil, false
		}
		encrypted, _ := data["encrypted"].(bool)
		// A live edit is checked against when it arrived if the server
		// leaves its time out: a zero time would skip the replay window.
		at := eventTime(data)
		if at.IsZero() {
			at = time.Now()
		}
		return messageUpdatedMsg{messageID: id, content: content, encrypted: encrypted, at: at}, true

	case apisdk.MessageDeleted:
		id, ok := stringField(data, "id", "ID", "message_id", "messageId")
		if !ok {
			log.Printf("Invalid message deleted payload: %+v", data)
			return nil, false
		}
		return messageDeletedMsg{messageID: id}, true

	case apisdk.MemberJoined:
		member, ok := parseMember(data)
		if !ok {
			log.Printf("Invalid member joined payload: %+v", data)
			return nil, false
		}
		return memberJoinedMsg{member: member}, true

	case apisdk.MemberLeft:
		member, ok := parseMember(data)
		if !ok {
			log.Printf("Invalid member left payload: %+v", data)
			return nil, false
		}
		return memberLeftMsg{userID: member.ID, username: member.Username}, true

	case apisdk.MemberList:
		raw, ok := data["members"].([]any)
		if !ok {
			log.Printf("Invalid member list payload: %+v", data)
			return nil, false
		}
		members := make([]apisdk.UserResponse, 0, len(raw))
		for _, r := range raw {
			fields, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if member, ok := parseMember(fields); ok {
				members = append(members, member)
			} else {
				log.Printf("Skipping invalid member in list: %+v", fields)
			}
		}
		return memberListMsg{members: members}, true

	case apisdk.Kicked:
		userID, okUserID := stringField(data, "userId", "userID")
		username, okUsername := stringField(data, "username", "Username")
		if !okUserID || !okUsername {
			log.Printf("Invalid kicked payload: %+v (missing username)", data)
			return nil, false
		}
		reason, _ := stringField(data, "reason", "Reason")
		if reason == "" {
			reason = "Removed by room owner"
		}
		return kickedMsg{userID: userID, username: username, reason: reason}, true

	case apisdk.RoomUpdated:
		joinCode, ok := stringField(data, "joinCode", "JoinCode")
		if !ok {
			return nil, false
		}
		return roomUpdatedMsg{joinCode: joinCode}, true

	case apisdk.ErrorEvent, apisdk.AuthenticationError, apisdk.JoinFailed, apisdk.RateLimited:
		code, okCode := stringField(data, "code", "Code")
		message, okMessage := stringField(data, "message", "Message")
		if !okCode || !okMessage {
			log.Printf("Invalid error payload: %+v", data)
			return roomErrorMsg{code: "UNKNOWN_ERROR", message: "An unknown error occurred"}, true
		}
		return roomErrorMsg{code: code, message: message}, true
	}
	return nil, false
}

// parseReactions reads messageID's reaction list, decrypting each emoji.
func parseReactions(raw any, messageID string, decrypt decryptFunc) []apisdk.Reaction {
	list, _ := raw.([]any)
	var reactions []apisdk.Reaction
	for _, r := range list {
		fields, ok := r.(map[string]any)
		if !ok {
			continue
		}
		userID, okID := stringField(fields, "userId")
		emoji, okEmoji := stringField(fields, "emoji")
		if !okID || !okEmoji {
			continue
		}
		encrypted, _ := fields["encrypted"].(bool)
		reactions = append(reactions, apisdk.Reaction{
			UserID: userID,
			Emoji:  decrypt(emoji, encrypted, sealedAs{reaction: true, from: userID, messageID: messageID}).text,
		})
	}
	return reactions
}

// parseMember reads a member's ID and username.
func parseMember(data map[string]any) (apisdk.UserResponse, bool) {
	userID, okID := stringField(data, "userId", "UserID", "user_id")
	username, okUsername := stringField(data, "username", "Username")
	return apisdk.UserResponse{ID: userID, Username: username}, okID && okUsername
}

// stringField returns the first non-empty string among data's keys, without
// control characters: these payloads come from the server (and usernames from
// other members), and are shown in the terminal.
func stringField(data map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := data[k].(string); ok && v != "" {
			return strfn.CleanLine(v), true
		}
	}
	return "", false
}

// eventTime returns the event's "timestamp", or the zero time if it has
// none.
func eventTime(data map[string]any) time.Time {
	ts, ok := stringField(data, "timestamp")
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}

// sealedAs is what an encrypted field claims to be: a message (or a reaction
// to messageID) written by from. Decryption checks the signed envelope
// matches, so neither the server nor another member can relabel content
// (see apisdk.RoomCrypto).
type sealedAs struct {
	reaction  bool
	edit      bool // an edit, sealed for messageID
	from      string
	messageID string
	// at is when the server says the content was sent (or, for an edit,
	// made); zero if unknown.
	at time.Time
	// updated is when the server says a message was last edited (zero if
	// never): history shows the latest edit as the content.
	updated time.Time
}

// decrypted is an encrypted field's verified text, or the placeholder shown
// instead when it failed.
type decrypted struct {
	text string
	// fileHash is the hash of the file an image message points to.
	fileHash string
}

// decryptFunc decrypts a field that should be sealed as want.
type decryptFunc func(content string, encrypted bool, want sealedAs) decrypted

// decryptContent decrypts and verifies an encrypted field with the room's
// crypto state; failures are shown in place of the content.
func decryptContent(rc *apisdk.RoomCrypto, content string, encrypted bool, want sealedAs) decrypted {
	if rc == nil {
		return decrypted{text: strfn.CleanText(content)}
	}
	if !encrypted {
		// Every member of an encrypted room encrypts: plaintext can only
		// have been injected by the server.
		return decrypted{text: apisdk.FailedText(apisdk.ErrUnverified)}
	}
	var (
		opened apisdk.Opened
		err    error
	)
	switch {
	case want.reaction:
		opened.Text, err = rc.OpenReaction(content, want.from, want.messageID)
	case want.edit:
		opened, err = rc.OpenEdit(content, want.from, want.messageID, want.at)
	default:
		opened, err = rc.OpenMessage(content, want.from, want.messageID, want.at, want.updated)
	}
	if err != nil {
		log.Printf("Failed to decrypt message: %v", err)
		return decrypted{text: apisdk.FailedText(err)}
	}
	// Members write the text: never let it reach the terminal as escape
	// sequences.
	text := strfn.CleanText(opened.Text)
	if want.reaction {
		text = strfn.CleanLine(text)
	}
	return decrypted{text: text, fileHash: opened.FileHash}
}
