package websocket

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type NotificationCore struct {
	clients    map[string]*NotificationClient // userID -> client
	register   chan *NotificationClient
	unregister chan *NotificationClient
	done       chan struct{}
	mu         sync.RWMutex
	upgrader   websocket.Upgrader
}

// NewNotificationCore creates the notification hub. checkOrigin validates the
// Origin of upgrade requests (see NewOriginChecker).
func NewNotificationCore(checkOrigin func(r *http.Request) bool) *NotificationCore {
	return &NotificationCore{
		clients:    make(map[string]*NotificationClient),
		register:   make(chan *NotificationClient),
		unregister: make(chan *NotificationClient),
		done:       make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     checkOrigin,
		},
	}
}

func (nc *NotificationCore) Run(ctx context.Context) {
	defer nc.cleanup()

	for {
		select {
		case <-ctx.Done():
			log.Println("NotificationCore shutting down...")
			return

		case client := <-nc.register:
			nc.mu.Lock()
			if existing, exists := nc.clients[client.UserID]; exists && existing != client {
				// Replace the previous connection; closing its send channel makes
				// its writer send a close frame and exit.
				close(existing.send)
			}
			nc.clients[client.UserID] = client
			total := len(nc.clients)
			nc.mu.Unlock()
			log.Printf("User %s registered for notifications (total: %d)", client.UserID, total)

		case client := <-nc.unregister:
			nc.mu.Lock()
			// Only remove the entry if it still belongs to this connection: a
			// replaced connection must not unregister its successor (and must
			// not close its own, already closed, send channel twice).
			if current, exists := nc.clients[client.UserID]; exists && current == client {
				delete(nc.clients, client.UserID)
				close(client.send)
			}
			total := len(nc.clients)
			nc.mu.Unlock()
			log.Printf("User %s unregistered from notifications (total: %d)", client.UserID, total)
		}
	}
}

func (nc *NotificationCore) NotifyUser(userID string, message *NotificationMessage) {
	// The send happens under the read lock: send channels are only closed
	// while holding the write lock, so this can never hit a closed channel.
	nc.mu.RLock()
	defer nc.mu.RUnlock()

	client, exists := nc.clients[userID]
	if !exists {
		log.Printf("User %s not connected to notification stream", userID)
		return
	}

	select {
	case client.send <- message:
		log.Printf("Notification sent to user %s", userID)
	default:
		log.Printf("Failed to send notification to user %s: channel full", userID)
	}
}

func (nc *NotificationCore) Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return nc.upgrader.Upgrade(w, r, nil)
}

// RegisterClient registers client. It returns false if the hub has shut down.
func (nc *NotificationCore) RegisterClient(client *NotificationClient) bool {
	select {
	case nc.register <- client:
		return true
	case <-nc.done:
		return false
	}
}

// UnregisterClient unregisters client. It never blocks after shutdown.
func (nc *NotificationCore) UnregisterClient(client *NotificationClient) {
	select {
	case nc.unregister <- client:
	case <-nc.done:
	}
}

func (nc *NotificationCore) cleanup() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	close(nc.done)

	for userID, client := range nc.clients {
		close(client.send)
		_ = client.conn.Close()
		log.Printf("Closed notification connection for user %s", userID)
	}
	nc.clients = make(map[string]*NotificationClient)
	log.Println("NotificationCore cleanup completed")
}
