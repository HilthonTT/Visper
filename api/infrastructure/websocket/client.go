package websocket

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn     *connWrapper
	Message  chan *WSMessage
	ID       string `json:"id"`
	RoomID   string `json:"roomId"`
	Username string `json:"username"`

	// Protection against double-close and race conditions
	closeOnce sync.Once
	closed    chan struct{} // signals when client is closed
	mu        sync.RWMutex
}

func NewClient(conn *websocket.Conn, id, roomID, username string) *Client {
	return &Client{
		conn:     newConnWrapper(conn),
		Message:  make(chan *WSMessage, 64),
		ID:       id,
		RoomID:   roomID,
		Username: username,
		closed:   make(chan struct{}),
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mu.Lock()
		_ = c.conn.Close()
		c.mu.Unlock()
		close(c.Message)
	})
}

func (c *Client) IsClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *Client) ReadMessage(core *Core) {
	defer func() {
		core.Unregister() <- c
		c.Close()
	}()

	_ = c.conn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	c.conn.conn.SetPongHandler(func(string) error {
		_ = c.conn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-c.closed:
			return
		default:
		}

		_, raw, err := c.conn.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error (client %s): %v", c.ID, err)
			}
			return
		}

		if len(raw) == 0 {
			continue
		}

		if len(raw) > 32768 { // 32KB max message size
			log.Printf("message too large from client %s: %d bytes", c.ID, len(raw))
			continue
		}

		now := time.Now().Format(time.RFC3339)

		payload := struct {
			Content   string `json:"content"`
			Username  string `json:"username"`
			Timestamp string `json:"timestamp"`
			UserID    string `json:"userId"`
			Encrypted bool   `json:"encrypted"`
		}{
			Content:   string(raw),
			Username:  c.Username,
			Timestamp: now,
			UserID:    c.ID,
			Encrypted: false,
		}

		msg := &WSMessage{
			Type:   MessageReceived,
			RoomID: c.RoomID,
			Data:   payload,
		}

		select {
		case core.Broadcast() <- msg:
		case <-c.closed:
			return
		}
	}
}

func (c *Client) WriteMessage() {
	defer c.Close()

	// Ping ticker to keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.Message:
			if !ok {
				// Channel closed, connection shutting down
				c.mu.Lock()
				_ = c.conn.conn.WriteMessage(websocket.CloseMessage, []byte{})
				c.mu.Unlock()
				return
			}

			// Set write deadline
			_ = c.conn.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			c.mu.Lock()
			err := c.conn.WriteJSON(msg)
			c.mu.Unlock()

			if err != nil {
				log.Printf("ws write error (client %s): %v", c.ID, err)
				return
			}

		case <-ticker.C:
			// Send ping
			c.mu.Lock()
			_ = c.conn.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := c.conn.conn.WriteMessage(websocket.PingMessage, nil)
			c.mu.Unlock()

			if err != nil {
				log.Printf("ping error (client %s): %v", c.ID, err)
				return
			}

		case <-c.closed:
			return
		}
	}
}
