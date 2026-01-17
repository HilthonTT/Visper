package tui

import (
	"context"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
)

type wsConnectedMsg struct {
	conn *apisdk.RoomWebSocket
}

type wsChannelReadyMsg struct {
	msgChan chan tea.Msg
}

type wsMessageReceivedMsg struct {
	message apisdk.MessageResponse
}

type wsMessageDeletedMsg struct {
	messageID string
}

type wsMessageUpdatedMsg struct {
	messageID string
	content   string
}

type wsMemberJoinedMsg struct {
	member apisdk.UserResponse
}

type wsMemberLeftMsg struct {
	userID   string
	username string
}

type wsMemberListMsg struct {
	members []apisdk.UserResponse
}

type wsKickedMsg struct {
	username string
	reason   string
}

type wsRoomDeletedMsg struct{}

type wsErrorMsg struct {
	code    string
	message string
}

type wsDisconnectedMsg struct{}

type wsKickTimeoutMsg struct{}

type wsRoomDeletedTimeoutMsg struct{}

func getStringField(data map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if val, ok := data[key].(string); ok && val != "" {
			return val, true
		}
	}
	return "", false
}

func (m model) connectWebSocket(roomID string) tea.Cmd {
	return func() tea.Msg {
		var userID string
		if m.userID != nil {
			userID = *m.userID
		}

		opts := []option.RequestOption{}
		if userID != "" {
			opts = append(opts, option.WithHeader("X-User-ID", userID))
		}

		ws, err := m.client.Room.ConnectWebSocket(m.context, roomID, opts...)
		if err != nil {
			log.Printf("Failed to connect WebSocket: %v", err)
			return wsErrorMsg{
				code:    "CONNECTION_FAILED",
				message: "Failed to connect to chat room",
			}
		}

		return wsConnectedMsg{conn: ws}
	}
}

func (m model) listenWebSocket() tea.Cmd {
	return func() tea.Msg {
		msgChan := make(chan tea.Msg, 100)

		m.state.chat.wsConn.SetMessageHandler(func(wsMsg apisdk.WSMessage) {
			select {
			case <-m.state.chat.wsCtx.Done():
				return // Context cancelled, don't process message
			default:
				// Continue processing
			}

			switch wsMsg.Type {
			case apisdk.MessageReceived:
				// Parse message payload with safe extraction
				if data, ok := wsMsg.Data.(map[string]any); ok {
					id, okID := getStringField(data, "id", "ID")
					userID, okUserID := getStringField(data, "userId", "UserID", "user_id")
					username, okUsername := getStringField(data, "username", "Username")
					content, okContent := getStringField(data, "content", "Content")

					if okID && okUserID && okUsername && okContent {
						msg := apisdk.MessageResponse{
							ID:       id,
							RoomID:   wsMsg.RoomID,
							UserID:   userID,
							Username: username,
							Content:  content,
						}

						select {
						case msgChan <- wsMessageReceivedMsg{message: msg}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid message received payload: %+v (missing fields)", data)
					}
				}

			case apisdk.MessageUpdated:
				if data, ok := wsMsg.Data.(map[string]any); ok {
					id, idOk := getStringField(data, "id", "ID")
					content, contentOk := getStringField(data, "content", "content")
					if idOk && contentOk {
						select {
						case msgChan <- wsMessageUpdatedMsg{messageID: id, content: content}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid message updated payload: %+v", data)
					}
				}

			case apisdk.MessageDeleted:
				if payload, ok := wsMsg.Data.(apisdk.MessageDeletedPayload); ok {
					select {
					case msgChan <- wsMessageDeletedMsg{messageID: payload.ID}:
					case <-m.state.chat.wsCtx.Done():
						return
					}
				} else if data, ok := wsMsg.Data.(map[string]any); ok {
					id, ok := getStringField(data, "id", "ID", "message_id", "messageId")
					if ok {
						select {
						case msgChan <- wsMessageDeletedMsg{messageID: id}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid message deleted payload: %+v", data)
					}
				} else {
					log.Printf("Unknown message deleted payload type: %T - %+v", wsMsg.Data, wsMsg.Data)
				}

			case apisdk.MemberJoined:
				if data, ok := wsMsg.Data.(map[string]any); ok {
					userID, okID := getStringField(data, "userId", "UserID", "user_id")
					username, okUsername := getStringField(data, "username", "Username")

					if okID && okUsername {
						member := apisdk.UserResponse{
							ID:       userID,
							Username: username,
						}

						select {
						case msgChan <- wsMemberJoinedMsg{member: member}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid member joined payload: %+v", data)
					}
				}

			case apisdk.MemberLeft:
				if data, ok := wsMsg.Data.(map[string]any); ok {
					userID, okID := getStringField(data, "userId", "UserID", "user_id")
					username, okUsername := getStringField(data, "username", "Username")

					if okID && okUsername {
						select {
						case msgChan <- wsMemberLeftMsg{
							userID:   userID,
							username: username,
						}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid member left payload: %+v", data)
					}
				}

			case apisdk.MemberList:
				if data, ok := wsMsg.Data.(map[string]any); ok {
					if membersData, ok := data["members"].([]any); ok {
						members := make([]apisdk.UserResponse, 0, len(membersData))
						for _, m := range membersData {
							if memberMap, ok := m.(map[string]any); ok {
								userID, okID := getStringField(memberMap, "userId", "UserID", "user_id")
								username, okUsername := getStringField(memberMap, "username", "Username")

								if okID && okUsername {
									members = append(members, apisdk.UserResponse{
										ID:       userID,
										Username: username,
									})
								} else {
									log.Printf("Skipping invalid member in list: %+v", memberMap)
								}
							}
						}

						select {
						case msgChan <- wsMemberListMsg{members: members}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid member list payload: %+v", data)
					}
				}

			case apisdk.Kicked:
				if data, ok := wsMsg.Data.(map[string]any); ok {
					username, okUsername := getStringField(data, "username", "Username")
					reason, okReason := getStringField(data, "reason", "Reason")

					if okUsername && okReason {
						select {
						case msgChan <- wsKickedMsg{
							username: username,
							reason:   reason,
						}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid kicked payload: %+v", data)
					}
				}

			case apisdk.RoomDeleted:
				select {
				case msgChan <- wsRoomDeletedMsg{}:
				case <-m.state.chat.wsCtx.Done():
					return
				}

			case apisdk.ErrorEvent, apisdk.AuthenticationError, apisdk.JoinFailed, apisdk.RateLimited:
				if data, ok := wsMsg.Data.(map[string]any); ok {
					code, okCode := getStringField(data, "code", "Code")
					message, okMessage := getStringField(data, "message", "Message")

					if okCode && okMessage {
						select {
						case msgChan <- wsErrorMsg{
							code:    code,
							message: message,
						}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					} else {
						log.Printf("Invalid error payload: %+v", data)
						// Send generic error message
						select {
						case msgChan <- wsErrorMsg{
							code:    "UNKNOWN_ERROR",
							message: "An unknown error occurred",
						}:
						case <-m.state.chat.wsCtx.Done():
							return
						}
					}
				}
			}
		})

		// Start listening in background
		go func() {
			defer func() {
				close(msgChan)
			}()

			if err := m.state.chat.wsConn.Listen(m.state.chat.wsCtx); err != nil {
				if err != context.Canceled {
					log.Printf("WebSocket listen error: %v", err)
					select {
					case msgChan <- wsDisconnectedMsg{}:
					case <-m.state.chat.wsCtx.Done():
					}
				}
			}
		}()

		return wsChannelReadyMsg{msgChan: msgChan}
	}
}

func waitForWSMessage(msgChan chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if msgChan == nil {
			return nil
		}
		msg, ok := <-msgChan
		if !ok {
			return wsDisconnectedMsg{}
		}
		return msg
	}
}
