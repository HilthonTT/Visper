package websocket

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

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
		Data:   data, // Changed from Payload
	}
}

func (c *NotificationClient) ReadMessage(core *NotificationCore) {
	defer func() {
		core.Unregister() <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
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
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("Failed to write notification to user %s: %v", c.UserID, err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *NotificationClient) Send() chan *NotificationMessage {
	return c.send
}
