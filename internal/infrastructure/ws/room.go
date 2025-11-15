package ws

import (
	"errors"
	"log"
	"sync"
)

var (
	ErrRoomNotFound   = errors.New("room not found")
	ErrClientNotFound = errors.New("client not found")
)

type WSRoom struct {
	ID      string             `json:"id"`
	Name    string             `json:"name,omitempty"`
	Clients map[string]*Client `json:"clients"`
	History []*WSMessage

	mu sync.RWMutex // protects History (and optionally Clients if you ever mutate outside RoomManager)
}

type RoomManager struct {
	rooms map[string]*WSRoom // roomID → WSRoom
	mu    sync.RWMutex
}

func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*WSRoom),
	}
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

	if _, exists := room.Clients[cl.ID]; !exists {
		room.Clients[cl.ID] = cl
	}
}

func (rm *RoomManager) RemoveClient(cl *Client) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if room, ok := rm.rooms[cl.RoomID]; ok {
		if _, ok := room.Clients[cl.ID]; ok {
			delete(room.Clients, cl.ID)
			close(cl.Message)

			if len(room.Clients) == 0 {
				delete(rm.rooms, cl.RoomID)
			}
		}
	}
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
	room.mu.Unlock()

	for _, cl := range room.Clients {
		select {
		case cl.Message <- msg:
		default:
			// Client is too slow – drop the message
			log.Printf("client %s buffer full, dropping message", cl.ID)
		}
	}
	return nil
}
