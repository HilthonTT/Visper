package apisdk

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

const (
	NotificationRoomInvite = "room_invite"
	NotificationError      = "notification.error"
)

type NotificationWSMessage struct {
	Type   string         `json:"type"`
	UserID string         `json:"userId"`
	Data   map[string]any `json:"data"`
}

type NotificationWebSocket struct {
	conn           *websocket.Conn
	userID         string
	mu             sync.RWMutex
	closed         bool
	messageHandler func(NotificationWSMessage)
	errorHandler   func(error)
}

func (ws *NotificationWebSocket) Close() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.closed {
		return nil
	}

	ws.closed = true
	return ws.conn.Close()
}

func (ws *NotificationWebSocket) SetMessageHandler(handler func(NotificationWSMessage)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.messageHandler = handler
}

func (ws *NotificationWebSocket) SetErrorHandler(handler func(error)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.errorHandler = handler
}

func (ws *NotificationWebSocket) Listen(ctx context.Context) error {
	defer ws.Close()

	log.Printf("[Notification WS] Started listening for user %s", ws.userID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Notification WS] Context done for user %s: %v", ws.userID, ctx.Err())
			return ctx.Err()
		default:
			var msg NotificationWSMessage
			err := ws.conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[Notification WS] Unexpected close error for user %s: %v", ws.userID, err)
					return fmt.Errorf("websocket read error: %w", err)
				}
				log.Printf("[Notification WS] Read error for user %s: %v", ws.userID, err)
				return err
			}

			log.Printf("[Notification WS] Received - Type: %s, UserID: %s, Data: %+v", msg.Type, msg.UserID, msg.Data)

			ws.mu.RLock()
			handler := ws.messageHandler
			ws.mu.RUnlock()

			if handler != nil {
				log.Printf("[Notification WS] Calling handler for type: %s", msg.Type)
				handler(msg)
			} else {
				log.Printf("[Notification WS] No handler set, dropping message type: %s", msg.Type)
			}
		}
	}
}

func (c *Client) ConnectNotificationWebSocket(
	ctx context.Context,
	opts ...option.RequestOption,
) (*NotificationWebSocket, error) {
	opts = append(c.Options, opts...)

	cfg, err := requestconfig.NewRequestConfig(ctx, http.MethodGet, "", nil, nil, opts...)
	if err != nil {
		return nil, err
	}

	if cfg.BaseURL == nil {
		return nil, fmt.Errorf("base URL is not configured")
	}

	baseURL := cfg.BaseURL.String()

	// Convert http(s) to ws(s)
	wsURL := baseURL
	if after, ok := strings.CutPrefix(baseURL, "https://"); ok {
		wsURL = "wss://" + after
	} else if after0, ok0 := strings.CutPrefix(baseURL, "http://"); ok0 {
		wsURL = "ws://" + after0
	}

	path := fmt.Sprintf("%s/api/v1/users/notifications/ws", wsURL)

	log.Printf("[NOTIFICATION_WEBSOCKET_URL]: %s\n", path)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	headers := http.Header{}
	for key, values := range cfg.Request.Header {
		for _, value := range values {
			headers.Add(key, value)
		}
	}

	if cfg.HTTPClient != nil {
		if jar := cfg.HTTPClient.Jar; jar != nil {
			if parsedURL, err := url.Parse(cfg.BaseURL.String()); err == nil {
				cookies := jar.Cookies(parsedURL)
				for _, cookie := range cookies {
					headers.Add("Cookie", cookie.String())
				}
			}
		}
	}

	conn, _, err := dialer.DialContext(ctx, path, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to notification websocket: %w", err)
	}

	// Extract userID from headers if available
	userID := cfg.Request.Header.Get("X-User-ID")

	ws := &NotificationWebSocket{
		conn:   conn,
		userID: userID,
	}

	return ws, nil
}
