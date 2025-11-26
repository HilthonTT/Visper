package ws

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn     *connWrapper
	Message  chan *WSMessage
	ID       string `json:"id"`
	RoomID   string `json:"roomId"`
	Username string `json:"username"`
}

func NewClient(conn *websocket.Conn, id, roomID, username string) *Client {
	return &Client{
		conn:     newConnWrapper(conn),
		Message:  make(chan *WSMessage, 64), // buffered to avoid dead-locks on slow clients
		ID:       id,
		RoomID:   roomID,
		Username: username,
	}
}

func (c *Client) ReadMessage(core *Core) {
	defer func() {
		core.Unregister() <- c
		_ = c.conn.Close()
	}()

	for {
		_, raw, err := c.conn.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error (client %s): %v", c.ID, err)
			}
			break
		}

		now := time.Now().Format(time.RFC3339)

		payload := struct {
			Content   string `json:"content"`
			Username  string `json:"username"`
			Timestamp string `json:"timestamp"`
			UserID    string `json:"userId"`
		}{
			Content:   string(raw),
			Username:  c.Username,
			Timestamp: now,
			UserID:    c.ID,
		}

		msg := &WSMessage{
			Type:   MessageReceived,
			RoomID: c.RoomID,
			Data:   payload,
		}

		core.Broadcast() <- msg
	}
}

func (c *Client) WriteMessage() {
	defer func() {
		_ = c.conn.Close()
	}()

	for msg := range c.Message {
		if err := c.conn.WriteJSON(msg); err != nil {
			log.Printf("ws write error (client %s): %v", c.ID, err)
			break
		}
	}
}
