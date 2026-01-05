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

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

type WSRoom struct {
	ID      string             `json:"id"`
	Name    string             `json:"name,omitempty"`
	Clients map[string]*Client `json:"clients"`
	History []*WSMessage

	mu sync.RWMutex
}

type RoomManager struct {
	rooms map[string]*WSRoom
	mu    sync.RWMutex
}

func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*WSRoom),
	}
}

func (rm *RoomManager) Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (rm *RoomManager) AddClient(cl *Client) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, ok := rm.rooms[cl.RoomID]
	if !ok {
		room = &WSRoom{
			ID:      cl.RoomID,
			Clients: make(map[string]*Client),
			History: make([]*WSMessage, 0, 64),
		}
		rm.rooms[cl.RoomID] = room
	}

	room.mu.Lock()
	if _, exists := room.Clients[cl.ID]; !exists {
		room.Clients[cl.ID] = cl
	}
	room.mu.Unlock()
}

func (rm *RoomManager) RemoveClient(cl *Client) {
	rm.mu.Lock()
	room, ok := rm.rooms[cl.RoomID]
	rm.mu.Unlock()

	if !ok {
		return
	}

	room.mu.Lock()
	if _, exists := room.Clients[cl.ID]; exists {
		delete(room.Clients, cl.ID)
		clientCount := len(room.Clients)
		room.mu.Unlock()

		if clientCount == 0 {
			rm.mu.Lock()
			room.mu.RLock()

			if len(room.Clients) == 0 {
				delete(rm.rooms, cl.RoomID)
			}
			room.mu.RUnlock()
			rm.mu.Unlock()
		}
	} else {
		room.mu.Unlock()
	}

	cl.Close()
}

func (rm *RoomManager) GetRoom(roomID string) (*WSRoom, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	r, ok := rm.rooms[roomID]
	return r, ok
}

func (rm *RoomManager) BroadcastToRoom(msg *WSMessage) error {
	rm.mu.RLock()
	room, ok := rm.rooms[msg.RoomID]
	rm.mu.RUnlock()

	if !ok {
		return ErrRoomNotFound
	}

	room.mu.Lock()
	room.History = append(room.History, msg)

	// Limit history size to prevent memory issues
	if len(room.History) > 1000 {
		room.History = room.History[len(room.History)-1000:]
	}
	room.mu.Unlock()

	// Create snapshot of clients to avoid holding lock during broadcast
	room.mu.RLock()
	clients := make([]*Client, 0, len(room.Clients))
	for _, cl := range room.Clients {
		clients = append(clients, cl)
	}
	room.mu.RUnlock()

	for _, cl := range clients {
		if cl.IsClosed() {
			continue
		}

		select {
		case cl.Message <- msg:
		default:
			// Client buffer full – drop message and log
			log.Printf("client %s buffer full, dropping message", cl.ID)
		}
	}

	return nil
}

func (rm *RoomManager) DisconnectAll() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, room := range rm.rooms {
		room.mu.Lock()
		for _, cl := range room.Clients {
			cl.Close()
		}
		room.mu.Unlock()
	}

	rm.rooms = make(map[string]*WSRoom)
}

func (rm *RoomManager) GetRoomStats(roomID string) (clientCount int, historySize int, exists bool) {
	rm.mu.RLock()
	room, ok := rm.rooms[roomID]
	rm.mu.RUnlock()

	if !ok {
		return 0, 0, false
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	return len(room.Clients), len(room.History), true
}
