package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	apisdk "github.com/hilthontt/visper/api-sdk"
)

type wsConnectedMsg struct {
	conn *apisdk.RoomWebSocket
}

type wsMessageReceivedMsg struct {
	message apisdk.MessageResponse
}

type wsMessageDeletedMsg struct {
	messageID string
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
	err     error
	code    string
	message string
	retry   bool
}

type wsKickTimeoutMsg struct{}

type wsRoomDeletedTimeoutMsg struct{}

type wsDisconnectedMsg struct{}

type wsChannelReadyMsg struct {
	msgChan chan tea.Msg
}

func (m model) connectWebSocket(roomID, joinCode, username string) tea.Cmd {
	return func() tea.Msg {
		wsConn, err := m.client.Room.ConnectWebSocket(
			context.Background(),
			apisdk.JoinRoomOpts{
				RoomID:   roomID,
				JoinCode: joinCode,
				Username: username,
			},
		)

		if err != nil {
			return wsErrorMsg{
				err:     err,
				message: "Failed to connect",
			}
		}

		return wsConnectedMsg{conn: wsConn}
	}
}

func (m model) listenWebSocket() tea.Cmd {
	ws := m.state.chat.wsConn
	ctx := m.state.chat.wsCtx
	msgChan := make(chan tea.Msg, 100)

	// Set up message handler
	ws.SetMessageHandler(func(msg apisdk.WSMessage) {
		teaMsg := m.handleWSMessage(msg)
		if teaMsg != nil {

			select {
			case msgChan <- teaMsg:

			default:

			}
		} else {

		}
	})

	// Start listening in a goroutine
	go func() {

		err := ws.Listen(ctx)
		if err != nil && err != context.Canceled {

			msgChan <- wsErrorMsg{
				err:     err,
				message: "Connection lost",
			}
		}

		msgChan <- wsDisconnectedMsg{}
		close(msgChan)
	}()

	// Return a command that both stores the channel AND starts waiting
	return func() tea.Msg {

		return wsChannelReadyMsg{msgChan: msgChan}
	}
}

func waitForWSMessage(msgChan chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-msgChan
		if !ok {
			return wsDisconnectedMsg{}
		}

		return msg
	}
}

func (m model) handleWSMessage(msg apisdk.WSMessage) tea.Msg {
	switch msg.Type {
	case apisdk.MessageReceived:
		data, _ := json.Marshal(msg.Data)
		var payload apisdk.MessagePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}

		createdAt, err := time.Parse(time.RFC3339, payload.Timestamp)
		if err != nil {
			createdAt = time.Now()
		}

		result := wsMessageReceivedMsg{
			message: apisdk.MessageResponse{
				ID: payload.ID,
				User: apisdk.UserResponse{
					ID:   payload.UserID,
					Name: payload.Username,
				},
				Content:   payload.Content,
				CreatedAt: createdAt,
			},
		}
		return result

	case apisdk.MessageDeleted:
		data, _ := json.Marshal(msg.Data)
		var payload apisdk.MessageDeletedPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		return wsMessageDeletedMsg{messageID: payload.MessageID}

	case apisdk.MemberJoined:
		data, _ := json.Marshal(msg.Data)
		var payload apisdk.MemberPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		return wsMemberJoinedMsg{
			member: apisdk.UserResponse{
				ID:   payload.UserID,
				Name: payload.Username,
			},
		}

	case apisdk.MemberLeft:
		data, _ := json.Marshal(msg.Data)
		var payload apisdk.MemberPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		return wsMemberLeftMsg{
			userID:   payload.UserID,
			username: payload.Username,
		}

	case apisdk.MemberList:
		data, _ := json.Marshal(msg.Data)
		var payload apisdk.MemberListPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		members := make([]apisdk.UserResponse, len(payload.Members))
		for i, m := range payload.Members {
			members[i] = apisdk.UserResponse{
				ID:   m.UserID,
				Name: m.Username,
			}
		}
		return wsMemberListMsg{members: members}

	case apisdk.Kicked:
		data, _ := json.Marshal(msg.Data)
		var payload apisdk.BootPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		return wsKickedMsg{
			username: payload.Username,
			reason:   payload.Reason,
		}

	case apisdk.RoomDeleted:
		return wsRoomDeletedMsg{}

	case apisdk.ErrorEvent, apisdk.AuthenticationError, apisdk.JoinFailed, apisdk.RateLimited:
		data, _ := json.Marshal(msg.Data)
		var payload apisdk.ErrorPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return wsErrorMsg{
				err:     fmt.Errorf("unknown error"),
				message: "An error occurred",
			}
		}
		return wsErrorMsg{
			err:     fmt.Errorf("%s: %s", payload.Code, payload.Message),
			code:    payload.Code,
			message: payload.Message,
			retry:   payload.Retry,
		}

	default:
		return nil
	}
}
