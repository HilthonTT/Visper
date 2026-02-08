package websocket

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/hilthontt/visper/api/domain/repository"
)

type Core struct {
	roomMgr           *RoomManager
	register          chan *Client
	unregister        chan *Client
	broadcast         chan *WSMessage
	roomRepository    repository.RoomRepository
	messageRepository repository.MessageRepository

	shutdown chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
}

func NewCore(roomRepository repository.RoomRepository, messageRepository repository.MessageRepository) *Core {
	return &Core{
		roomMgr:           NewRoomManager(),
		register:          make(chan *Client),
		unregister:        make(chan *Client),
		broadcast:         make(chan *WSMessage, 256),
		roomRepository:    roomRepository,
		messageRepository: messageRepository,
		shutdown:          make(chan struct{}),
	}
}

func (c *Core) Run(ctx context.Context) {
	defer c.wg.Wait() // Wait for all goroutines to finish

	for {
		select {
		case <-ctx.Done():
			log.Println("Core shutting down...")
			c.Shutdown()
			return

		case <-c.shutdown:
			return

		case cl := <-c.register:
			c.roomMgr.AddClient(cl)

			// Load persisted history with proper error handling
			c.wg.Add(1)
			go func(client *Client) {
				defer c.wg.Done()
				c.loadHistory(client)
			}(cl)

		case cl := <-c.unregister:
			c.roomMgr.RemoveClient(cl)

		case msg := <-c.broadcast:
			if err := c.roomMgr.BroadcastToRoom(msg); err != nil {
				log.Printf("broadcast error: %v", err)
			}
		}
	}
}

func (c *Core) loadHistory(cl *Client) {
	if cl.IsClosed() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limit := int64(50)
	messages, err := c.messageRepository.GetByRoom(ctx, cl.RoomID, limit)
	if err != nil {
		log.Printf("failed to load history for room %s: %v", cl.RoomID, err)
		return
	}

	for _, m := range messages {
		if cl.IsClosed() {
			return
		}

		payload := struct {
			Content   string `json:"content"`
			Username  string `json:"username"`
			Timestamp string `json:"timestamp"`
			UserID    string `json:"userId"`
			Encrypted bool   `json:"encrypted"`
		}{
			Content:   m.Content,
			Username:  m.Username,
			Timestamp: m.CreatedAt.Format(time.RFC3339),
			UserID:    m.UserID,
			Encrypted: m.Encrypted,
		}

		hist := &WSMessage{
			Type:   MessageReceived,
			RoomID: cl.RoomID,
			Data:   payload,
		}

		select {
		case cl.Message <- hist:
		case <-time.After(5 * time.Second):
			log.Printf("timeout sending history to client %s", cl.ID)
			return
		case <-cl.closed:
			return
		}
	}
}

func (c *Core) Register() chan<- *Client {
	return c.register
}

func (c *Core) Unregister() chan<- *Client {
	return c.unregister
}

func (c *Core) Broadcast() chan<- *WSMessage {
	return c.broadcast
}

func (c *Core) Shutdown() {
	c.once.Do(func() {
		close(c.shutdown)

		close(c.register)
		close(c.unregister)
		close(c.broadcast)

		c.roomMgr.DisconnectAll()
	})
}
