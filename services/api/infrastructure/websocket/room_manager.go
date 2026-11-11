package websocket

import (
	"errors"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	ErrRoomNotFound   = errors.New("room not found")
	ErrClientNotFound = errors.New("client not found")
)

// WSRoom is the set of live connections of a room. A user may have several
// connections (tabs/devices), so clients are tracked per connection.
type WSRoom struct {
	ID      string
	clients map[*Client]struct{}
}

type RoomManager struct {
	upgrader websocket.Upgrader
	rooms    map[string]*WSRoom
	mu       sync.RWMutex
}

// NewRoomManager creates a room manager whose upgrader validates the request
// Origin with checkOrigin (see NewOriginChecker). A nil checkOrigin falls back
// to gorilla's same-origin policy.
func NewRoomManager(checkOrigin func(r *http.Request) bool) *RoomManager {
	return &RoomManager{
		upgrader: websocket.Upgrader{
			CheckOrigin:     checkOrigin,
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		rooms: make(map[string]*WSRoom),
	}
}

func (rm *RoomManager) Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return rm.upgrader.Upgrade(w, r, nil)
}

func (rm *RoomManager) AddClient(cl *Client) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, ok := rm.rooms[cl.RoomID]
	if !ok {
		room = &WSRoom{
			ID:      cl.RoomID,
			clients: make(map[*Client]struct{}),
		}
		rm.rooms[cl.RoomID] = room
	}

	room.clients[cl] = struct{}{}
}

func (rm *RoomManager) RemoveClient(cl *Client) {
	rm.mu.Lock()
	if room, ok := rm.rooms[cl.RoomID]; ok {
		delete(room.clients, cl)
		if len(room.clients) == 0 {
			delete(rm.rooms, cl.RoomID)
		}
	}
	rm.mu.Unlock()

	cl.Close()
}

// ClientCount returns the number of live connections in a room.
func (rm *RoomManager) ClientCount(roomID string) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if room, ok := rm.rooms[roomID]; ok {
		return len(room.clients)
	}
	return 0
}

func (rm *RoomManager) BroadcastToRoom(msg *WSMessage) error {
	rm.mu.RLock()
	room, ok := rm.rooms[msg.RoomID]
	if !ok {
		rm.mu.RUnlock()
		return ErrRoomNotFound
	}

	// Snapshot the clients to avoid holding the lock while sending.
	clients := make([]*Client, 0, len(room.clients))
	for cl := range room.clients {
		clients = append(clients, cl)
	}
	rm.mu.RUnlock()

	for _, cl := range clients {
		if !cl.send(msg) && !cl.IsClosed() {
			// Client buffer full – drop message and log
			log.Printf("client %s buffer full, dropping message", cl.ID)
		}
	}

	return nil
}

// DisconnectUser removes every connection of userID from the room. Messages
// already queued for them (such as a kick notice) are flushed before the
// connection is closed.
func (rm *RoomManager) DisconnectUser(roomID, userID string) {
	rm.mu.Lock()
	var toClose []*Client
	if room, ok := rm.rooms[roomID]; ok {
		for cl := range room.clients {
			if cl.ID == userID {
				delete(room.clients, cl)
				toClose = append(toClose, cl)
			}
		}
		if len(room.clients) == 0 {
			delete(rm.rooms, roomID)
		}
	}
	rm.mu.Unlock()

	for _, cl := range toClose {
		cl.CloseAfterFlush()
	}
}

// CloseRoom disconnects every client of the room after flushing their queues.
func (rm *RoomManager) CloseRoom(roomID string) {
	rm.mu.Lock()
	room, ok := rm.rooms[roomID]
	delete(rm.rooms, roomID)
	rm.mu.Unlock()

	if !ok {
		return
	}

	for cl := range room.clients {
		cl.CloseAfterFlush()
	}
}

func (rm *RoomManager) DisconnectAll() {
	rm.mu.Lock()
	rooms := rm.rooms
	rm.rooms = make(map[string]*WSRoom)
	rm.mu.Unlock()

	for _, room := range rooms {
		for cl := range room.clients {
			cl.Close()
		}
	}
}
