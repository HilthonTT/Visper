package ws

import (
	"context"
	"log"
	"time"

	"github.com/hilthontt/visper/internal/domain"
)

type Core struct {
	roomMgr           *RoomManager
	register          chan *Client
	unregister        chan *Client
	broadcast         chan *WSMessage
	roomRepository    domain.RoomRepository
	messageRepository domain.MessageRepository
}

func NewCore(roomRepository domain.RoomRepository, messageRepository domain.MessageRepository) *Core {
	return &Core{
		roomMgr:           NewRoomManager(),
		register:          make(chan *Client),
		unregister:        make(chan *Client),
		broadcast:         make(chan *WSMessage, 256),
		roomRepository:    roomRepository,
		messageRepository: messageRepository,
	}
}

func (c *Core) Run() {
	for {
		select {
		case cl := <-c.register:
			c.roomMgr.AddClient(cl)

			// ---------- Load persisted history ----------
			go func() {
				messages, err := c.messageRepository.GetByRoomID(context.Background(), cl.RoomID)
				if err != nil {
					log.Printf("room %s not in DB: %v", cl.RoomID, err)
					return
				}
				for _, m := range messages {
					payload := struct {
						Content   string `json:"content"`
						Username  string `json:"username"`
						Timestamp string `json:"timestamp"`
						UserID    string `json:"userId"`
					}{
						Content:   m.Content,
						Username:  m.User.Name,
						Timestamp: m.CreatedAt.Format(time.RFC3339),
						UserID:    m.User.ID,
					}

					hist := &WSMessage{
						Type:   MessageReceived,
						RoomID: cl.RoomID,
						Data:   payload,
					}
					cl.Message <- hist
				}
			}()

		case cl := <-c.unregister:
			c.roomMgr.RemoveClient(cl)

		case msg := <-c.broadcast:
			if err := c.roomMgr.BroadcastToRoom(msg); err != nil {
				log.Printf("broadcast error: %v", err)
			}
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
