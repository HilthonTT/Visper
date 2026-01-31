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
	mu         sync.RWMutex
	upgrader   websocket.Upgrader
}

func NewNotificationCore() *NotificationCore {
	return &NotificationCore{
		clients:    make(map[string]*NotificationClient),
		register:   make(chan *NotificationClient),
		unregister: make(chan *NotificationClient),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
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
			if existingClient, exists := nc.clients[client.UserID]; exists {
				close(existingClient.send)
			}
			nc.clients[client.UserID] = client
			nc.mu.Unlock()
			log.Printf("User %s registered for notifications (total: %d)", client.UserID, len(nc.clients))

		case client := <-nc.unregister:
			nc.mu.Lock()
			if _, exists := nc.clients[client.UserID]; exists {
				delete(nc.clients, client.UserID)
				close(client.send)
			}
			nc.mu.Unlock()
			log.Printf("User %s unregistered from notifications (total: %d)", client.UserID, len(nc.clients))
		}
	}
}

func (nc *NotificationCore) NotifyUser(userID string, message *NotificationMessage) {
	nc.mu.RLock()
	client, exists := nc.clients[userID]
	nc.mu.RUnlock()

	if exists {
		select {
		case client.send <- message:
			log.Printf("Notification sent to user %s", userID)
		default:
			log.Printf("Failed to send notification to user %s: channel full", userID)
		}
	} else {
		log.Printf("User %s not connected to notification stream", userID)
	}
}

func (nc *NotificationCore) Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return nc.upgrader.Upgrade(w, r, nil)
}

func (nc *NotificationCore) Register() chan *NotificationClient {
	return nc.register
}

func (nc *NotificationCore) Unregister() chan *NotificationClient {
	return nc.unregister
}

func (nc *NotificationCore) cleanup() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	for userID, client := range nc.clients {
		close(client.send)
		client.conn.Close()
		log.Printf("Closed notification connection for user %s", userID)
	}
	nc.clients = make(map[string]*NotificationClient)
	log.Println("NotificationCore cleanup completed")
}
