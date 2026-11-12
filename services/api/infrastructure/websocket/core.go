package websocket

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/hilthontt/visper/services/api/domain/repository"
)

const historyLimit = 50

// Core is the hub for room websocket clients. All registration, removal and
// broadcasting is serialised through Run.
type Core struct {
	roomMgr           *RoomManager
	register          chan *Client
	unregister        chan *Client
	broadcast         chan *WSMessage
	messageRepository repository.MessageRepository

	// reads is each room's last-read message per member. It is kept in
	// memory, like the connections themselves, and dropped with the room.
	readsMu sync.Mutex
	reads   map[string]map[string]string // room ID -> user ID -> message ID

	// shutdown is closed when Run stops; the channels above are never closed
	// so that late senders cannot panic, they observe shutdown instead.
	shutdown chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
}

func NewCore(roomMgr *RoomManager, messageRepository repository.MessageRepository) *Core {
	return &Core{
		roomMgr:           roomMgr,
		register:          make(chan *Client),
		unregister:        make(chan *Client),
		broadcast:         make(chan *WSMessage, 256),
		messageRepository: messageRepository,
		reads:             make(map[string]map[string]string),
		shutdown:          make(chan struct{}),
	}
}

func (c *Core) Run(ctx context.Context) {
	defer c.wg.Wait() // Wait for history loaders to finish

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
			c.dispatch(NewPresenceUpdate(cl.RoomID, c.roomMgr.OnlineUsers(cl.RoomID)))

			c.wg.Add(1)
			go func(client *Client) {
				defer c.wg.Done()
				c.loadHistory(client)
			}(cl)

		case cl := <-c.unregister:
			c.roomMgr.RemoveClient(cl)
			c.dispatch(NewPresenceUpdate(cl.RoomID, c.roomMgr.OnlineUsers(cl.RoomID)))

		case msg := <-c.broadcast:
			c.dispatch(msg)
		}
	}
}

// dispatch delivers msg to the room and applies its side effects on
// connections: users that were kicked or left are disconnected (after the
// notice has been flushed to them) and a deleted room is closed.
func (c *Core) dispatch(msg *WSMessage) {
	if err := c.roomMgr.BroadcastToRoom(msg); err != nil && !errors.Is(err, ErrRoomNotFound) {
		log.Printf("broadcast error: %v", err)
	}

	switch msg.Type {
	case Kicked:
		if p, ok := msg.Data.(ErrorKickedPayload); ok {
			c.roomMgr.DisconnectUser(msg.RoomID, p.UserID)
		}
	case MemberLeft:
		if p, ok := msg.Data.(MemberPayload); ok {
			c.roomMgr.DisconnectUser(msg.RoomID, p.UserID)
		}
	case RoomDeleted:
		c.roomMgr.CloseRoom(msg.RoomID)
		c.readsMu.Lock()
		delete(c.reads, msg.RoomID)
		c.readsMu.Unlock()
	}
}

func (c *Core) loadHistory(cl *Client) {
	if cl.IsClosed() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	messages, err := c.messageRepository.GetByRoom(ctx, cl.RoomID, historyLimit)
	if err != nil {
		log.Printf("failed to load history for room %s: %v", cl.RoomID, err)
		return
	}

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	replay := make([]*WSMessage, 0, len(messages))
	for _, m := range messages {
		replay = append(replay, NewMessageReceived(cl.RoomID, m))
	}
	// Then where everyone has read up to.
	for userID, messageID := range c.readsOf(cl.RoomID) {
		replay = append(replay, NewReadUpdate(cl.RoomID, userID, messageID))
	}

	for _, hist := range replay {
		select {
		case cl.Message <- hist:
		case <-timeout.C:
			log.Printf("timeout sending history to client %s", cl.ID)
			return
		case <-cl.closed:
			return
		case <-c.shutdown:
			return
		}
	}
}

// RegisterClient adds cl to its room. It returns false if the hub has shut down.
// MarkRead records the last message userID has read in a room and tells the
// room.
func (c *Core) MarkRead(roomID, userID, messageID string) bool {
	c.readsMu.Lock()
	room, ok := c.reads[roomID]
	if !ok {
		room = make(map[string]string)
		c.reads[roomID] = room
	}
	changed := room[userID] != messageID
	room[userID] = messageID
	c.readsMu.Unlock()

	if !changed {
		return true
	}
	return c.Publish(NewReadUpdate(roomID, userID, messageID))
}

// readsOf returns a copy of a room's read markers.
func (c *Core) readsOf(roomID string) map[string]string {
	c.readsMu.Lock()
	defer c.readsMu.Unlock()
	out := make(map[string]string, len(c.reads[roomID]))
	for userID, messageID := range c.reads[roomID] {
		out[userID] = messageID
	}
	return out
}

func (c *Core) RegisterClient(cl *Client) bool {
	select {
	case c.register <- cl:
		return true
	case <-c.shutdown:
		return false
	}
}

// UnregisterClient removes cl from its room. It never blocks after shutdown.
func (c *Core) UnregisterClient(cl *Client) {
	select {
	case c.unregister <- cl:
	case <-c.shutdown:
	}
}

// Publish queues msg for delivery to its room. It returns false if the hub
// has shut down.
func (c *Core) Publish(msg *WSMessage) bool {
	// Checked first: with buffer space available a single select would pick
	// randomly between the send and the closed shutdown channel.
	select {
	case <-c.shutdown:
		return false
	default:
	}

	select {
	case c.broadcast <- msg:
		return true
	case <-c.shutdown:
		return false
	}
}

func (c *Core) Shutdown() {
	c.once.Do(func() {
		close(c.shutdown)
		c.roomMgr.DisconnectAll()
	})
}
