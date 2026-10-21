package websocket

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10

	// maxClientMessageSize bounds a single frame read from a room client.
	maxClientMessageSize = 32 * 1024

	clientSendBuffer = 64
)

type Client struct {
	conn *websocket.Conn

	// Message is the outbound queue. It is never closed: senders must not
	// block on it and the writer stops on closed/drain instead, so a
	// concurrent send can never panic.
	Message  chan *WSMessage
	ID       string `json:"id"`
	RoomID   string `json:"roomId"`
	Username string `json:"username"`

	closeOnce sync.Once
	closed    chan struct{} // closed when the connection is torn down

	drainOnce sync.Once
	drain     chan struct{} // closed to flush queued messages and then close
}

func NewClient(conn *websocket.Conn, id, roomID, username string) *Client {
	return &Client{
		conn:     conn,
		Message:  make(chan *WSMessage, clientSendBuffer),
		ID:       id,
		RoomID:   roomID,
		Username: username,
		closed:   make(chan struct{}),
		drain:    make(chan struct{}),
	}
}

// Close tears the connection down immediately. It is safe to call
// concurrently and more than once.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

// CloseAfterFlush asks the writer to deliver the messages already queued
// (e.g. a "kicked" notice), send a close frame and then close the connection.
func (c *Client) CloseAfterFlush() {
	c.drainOnce.Do(func() {
		close(c.drain)
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

func (c *Client) isDraining() bool {
	select {
	case <-c.drain:
		return true
	default:
		return false
	}
}

// send queues msg without blocking. It reports false when the client is gone
// or its buffer is full.
func (c *Client) send(msg *WSMessage) bool {
	if c.IsClosed() {
		return false
	}

	select {
	case c.Message <- msg:
		return true
	default:
		return false
	}
}

func (c *Client) ReadMessage(core *Core) {
	defer func() {
		core.UnregisterClient(c)
		c.Close()
	}()

	c.conn.SetReadLimit(maxClientMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error (client %s): %v", c.ID, err)
			}
			return
		}

		// A client that was kicked or left must not be able to keep posting.
		if c.IsClosed() || c.isDraining() {
			return
		}

		if len(raw) == 0 {
			continue
		}

		payload := struct {
			Content   string `json:"content"`
			Username  string `json:"username"`
			Timestamp string `json:"timestamp"`
			UserID    string `json:"userId"`
			Encrypted bool   `json:"encrypted"`
		}{
			Content:   string(raw),
			Username:  c.Username,
			Timestamp: time.Now().Format(time.RFC3339),
			UserID:    c.ID,
			Encrypted: false,
		}

		if !core.Publish(&WSMessage{
			Type:   MessageReceived,
			RoomID: c.RoomID,
			Data:   payload,
		}) {
			return
		}
	}
}

func (c *Client) WriteMessage() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case msg := <-c.Message:
			if err := c.write(msg); err != nil {
				log.Printf("ws write error (client %s): %v", c.ID, err)
				return
			}

		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				log.Printf("ping error (client %s): %v", c.ID, err)
				return
			}

		case <-c.drain:
			c.flush()
			_ = c.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(writeWait),
			)
			return

		case <-c.closed:
			return
		}
	}
}

// flush writes every message that is already queued.
func (c *Client) flush() {
	for {
		select {
		case msg := <-c.Message:
			if err := c.write(msg); err != nil {
				return
			}
		default:
			return
		}
	}
}

// write is only called from the WriteMessage goroutine, which is the single
// writer of data frames for this connection.
func (c *Client) write(msg *WSMessage) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteJSON(msg)
}
