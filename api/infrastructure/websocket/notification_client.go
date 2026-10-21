package websocket

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// maxNotificationMessageSize bounds frames read from notification clients;
// they are not expected to send anything but control frames.
const maxNotificationMessageSize = 512

type NotificationClient struct {
	conn     *websocket.Conn
	send     chan *NotificationMessage
	UserID   string
	Username string
}

type NotificationMessage struct {
	Type   string         `json:"type"`
	UserID string         `json:"userId"`
	Data   map[string]any `json:"data"`
}

func NewNotificationClient(conn *websocket.Conn, userID, username string) *NotificationClient {
	return &NotificationClient{
		conn:     conn,
		send:     make(chan *NotificationMessage, 256),
		UserID:   userID,
		Username: username,
	}
}

func NewNotificationMessage(msgType, userID string, data map[string]any) *NotificationMessage {
	return &NotificationMessage{
		Type:   msgType,
		UserID: userID,
		Data:   data,
	}
}

func (c *NotificationClient) ReadMessage(core *NotificationCore) {
	defer func() {
		core.UnregisterClient(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxNotificationMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Notification WebSocket error for user %s: %v", c.UserID, err)
			}
			break
		}
		// We don't expect messages from client, just keep connection alive
	}
}

func (c *NotificationClient) WriteMessage() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("Failed to write notification to user %s: %v", c.UserID, err)
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *NotificationClient) Send() chan *NotificationMessage {
	return c.send
}
