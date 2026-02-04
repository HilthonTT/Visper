package tui

import (
	"context"
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
)

type notificationWSConnectedMsg struct {
	conn *apisdk.NotificationWebSocket
}

type notificationWSChannelReadyMsg struct {
	msgChan chan tea.Msg
}

type notificationWSRoomInviteMsg struct {
	roomID     string
	joinCode   string
	secureCode string
	timestamp  int64
	expiresAt  time.Time
}

type notificationWSErrorMsg struct {
	code    string
	message string
}

type notificationWSDisconnectedMsg struct{}

type roomInviteAcceptedMsg struct{}
type roomInviteDeclinedMsg struct{}

func (m model) connectNotificationWebSocket() tea.Cmd {
	return func() tea.Msg {
		var userID string
		if m.userID != nil {
			userID = *m.userID
		}

		if userID == "" {
			log.Printf("Cannot connect notification WebSocket: no user ID")
			return nil
		}

		opts := []option.RequestOption{
			option.WithHeader("X-User-ID", userID),
		}

		ws, err := m.client.ConnectNotificationWebSocket(m.context, opts...)
		if err != nil {
			log.Printf("Failed to connect notification WebSocket: %v", err)
			return notificationWSErrorMsg{
				code:    "CONNECTION_FAILED",
				message: "Failed to connect to notification service",
			}
		}

		return notificationWSConnectedMsg{conn: ws}
	}
}

func (m model) listenNotificationWebSocket() tea.Cmd {
	return func() tea.Msg {
		msgChan := make(chan tea.Msg, 100)

		m.state.notification.wsConn.SetMessageHandler(func(wsMsg apisdk.NotificationWSMessage) {
			select {
			case <-m.state.notification.wsCtx.Done():
				return
			default:
			}

			log.Printf("[Handler] Received notification type: %s", wsMsg.Type)

			switch wsMsg.Type {
			case apisdk.NotificationRoomInvite:
				data := wsMsg.Data
				roomID, _ := data["room_id"].(string)
				joinCode, _ := data["join_code"].(string)
				secureCode, _ := data["secure_code"].(string)
				timestamp, _ := data["timestamp"].(float64)
				expiresAtStr, _ := data["expires_at"].(string)

				var expiresAt time.Time
				if expiresAtStr != "" {
					expiresAt, _ = time.Parse(time.RFC3339, expiresAtStr)
				}

				log.Printf("[Handler] Parsed: roomID=%s, joinCode=%s, secureCode=%s", roomID, joinCode, secureCode)

				if roomID != "" && joinCode != "" && secureCode != "" {
					msg := notificationWSRoomInviteMsg{
						roomID:     roomID,
						joinCode:   joinCode,
						secureCode: secureCode,
						timestamp:  int64(timestamp),
						expiresAt:  expiresAt,
					}
					log.Printf("[Handler] Sending to msgChan: %+v", msg)
					select {
					case msgChan <- msg:
						log.Println("[Handler] Successfully sent to msgChan")
					case <-m.state.notification.wsCtx.Done():
						log.Println("[Handler] Context done, aborting send")
						return
					}
				} else {
					log.Printf("Invalid room invite payload: %+v", data)
				}

			case apisdk.NotificationError:
				data := wsMsg.Data
				code, _ := data["code"].(string)
				message, _ := data["message"].(string)

				select {
				case msgChan <- notificationWSErrorMsg{
					code:    code,
					message: message,
				}:
				case <-m.state.notification.wsCtx.Done():
					return
				}
			}
		})

		go func() {
			defer close(msgChan)

			if err := m.state.notification.wsConn.Listen(m.state.notification.wsCtx); err != nil {
				if err != context.Canceled {
					log.Printf("Notification WebSocket listen error: %v", err)
					select {
					case msgChan <- notificationWSDisconnectedMsg{}:
					case <-m.state.notification.wsCtx.Done():
					}
				}
			}
		}()

		return notificationWSChannelReadyMsg{msgChan: msgChan}
	}
}

func waitForNotificationWSMessage(msgChan chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if msgChan == nil {
			log.Println("[Wait] msgChan is nil")
			return nil
		}
		log.Println("[Wait] Waiting for message from msgChan...")
		msg, ok := <-msgChan
		if !ok {
			log.Println("[Wait] Channel closed")
			return notificationWSDisconnectedMsg{}
		}
		log.Printf("[Wait] Received message type: %T, value: %+v", msg, msg)
		return msg
	}
}
