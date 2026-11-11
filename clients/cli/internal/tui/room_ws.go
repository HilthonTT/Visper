package tui

import (
	"context"
	"errors"
	"log"
	"time"

	tea "charm.land/bubbletea/v2"
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
	messageUpdatedMsg struct {
		messageID string
		content   string
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
// delivered on, as a roomListeningMsg. encryptionKey decrypts the messages of
// an encrypted room.
func listenRoom(session context.Context, conn *apisdk.RoomWebSocket, encryptionKey string) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 100)
		send := func(msg tea.Msg) {
			select {
			case events <- msg:
			case <-session.Done():
			}
		}
		decrypt := func(content string, encrypted bool) string {
			return decryptContent(encryptionKey, content, encrypted)
		}

		conn.SetMessageHandler(func(wsMsg apisdk.WSMessage) {
			if session.Err() != nil {
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
func parseRoomEvent(wsMsg apisdk.WSMessage, decrypt func(content string, encrypted bool) string) (tea.Msg, bool) {
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
		createdAt := time.Now()
		if ts, ok := stringField(data, "timestamp"); ok {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				createdAt = t
			}
		}
		return messageReceivedMsg{message: apisdk.MessageResponse{
			ID:        id,
			RoomID:    wsMsg.RoomID,
			UserID:    userID,
			Username:  username,
			Content:   decrypt(content, encrypted),
			Encrypted: encrypted,
			CreatedAt: createdAt.Local(),
		}}, true

	case apisdk.MessageUpdated:
		id, okID := stringField(data, "id", "ID")
		content, okContent := stringField(data, "content", "Content")
		if !okID || !okContent {
			log.Printf("Invalid message updated payload: %+v", data)
			return nil, false
		}
		encrypted, _ := data["encrypted"].(bool)
		return messageUpdatedMsg{messageID: id, content: decrypt(content, encrypted)}, true

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

// parseMember reads a member's ID and username.
func parseMember(data map[string]any) (apisdk.UserResponse, bool) {
	userID, okID := stringField(data, "userId", "UserID", "user_id")
	username, okUsername := stringField(data, "username", "Username")
	return apisdk.UserResponse{ID: userID, Username: username}, okID && okUsername
}

// stringField returns the first non-empty string among data's keys.
func stringField(data map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := data[k].(string); ok && v != "" {
			return v, true
		}
	}
	return "", false
}

// decryptContent decrypts an encrypted message with the room's key; failures
// are shown in place of the message.
func decryptContent(key, content string, encrypted bool) string {
	if !encrypted {
		return content
	}
	if key == "" {
		log.Printf("Cannot decrypt: no encryption key available")
		return "[Decryption failed: no key]"
	}
	decrypted, err := apisdk.DecryptWithKeyB64(content, key)
	if err != nil {
		log.Printf("Failed to decrypt message: %v", err)
		return "[Decryption failed]"
	}
	return decrypted
}
