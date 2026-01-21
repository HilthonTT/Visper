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
	MemberJoined = "member.joined"
	MemberLeft   = "member.left"
	MemberList   = "member.list"

	MessageReceived = "message.received"
	MessageDeleted  = "message.deleted"
	MessageUpdated  = "message.updated"

	ErrorEvent          = "error"
	AuthenticationError = "error.auth"
	JoinFailed          = "error.join"
	RateLimited         = "error.rate_limited"
	Kicked              = "error.kicked"

	RoomDeleted = "room.deleted"
	RoomUpdated = "room.updated"
)

type WSMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomId"`
	Data   any    `json:"data"`
}

type MessagePayload struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Timestamp string `json:"timestamp"`
}

type MessageDeletedPayload struct {
	ID string `json:"id"`
}

type MemberPayload struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	JoinedAt string `json:"joinedAt,omitempty"`
}

type MemberListPayload struct {
	Members []MemberPayload `json:"members"`
}

type ErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Retry   bool   `json:"retry,omitempty"`
}

type BootPayload struct {
	Username string `json:"username"`
	Reason   string `json:"reason"`
}

type RoomDeletedPayload struct {
	RoomID string `json:"roomid"`
}

type RoomWebSocket struct {
	conn           *websocket.Conn
	roomID         string
	username       string
	mu             sync.RWMutex
	closed         bool
	messageHandler func(WSMessage)
	errorHandler   func(error)
}

func (ws *RoomWebSocket) Close() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.closed {
		return nil
	}

	ws.closed = true
	return ws.conn.Close()
}

func (ws *RoomWebSocket) SetMessageHandler(handler func(WSMessage)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.messageHandler = handler
}

func (ws *RoomWebSocket) SetErrorHandler(handler func(error)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.errorHandler = handler
}

func (ws *RoomWebSocket) Listen(ctx context.Context) error {
	defer ws.Close()

	log.Printf("[WS] Started listening for messages on room %s", ws.roomID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[WS] Context done for room %s: %v", ws.roomID, ctx.Err())
			return ctx.Err()
		default:
			var msg WSMessage
			err := ws.conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[WS] Unexpected close error for room %s: %v", ws.roomID, err)
					return fmt.Errorf("websocket read error: %w", err)
				}
				log.Printf("[WS] Read error for room %s: %v", ws.roomID, err)
				return err
			}

			log.Printf("[WS] Received message - Type: %s, RoomID: %s, Data: %+v", msg.Type, msg.RoomID, msg.Data)

			ws.mu.RLock()
			handler := ws.messageHandler
			ws.mu.RUnlock()

			if handler != nil {
				log.Printf("[WS] Calling message handler for type: %s", msg.Type)
				handler(msg)
			} else {
				log.Printf("[WS] No message handler set, dropping message type: %s", msg.Type)
			}
		}
	}
}

func (ws *RoomWebSocket) SendMessage(content string) error {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if ws.closed {
		return fmt.Errorf("websocket connection is closed")
	}

	return ws.conn.WriteMessage(websocket.TextMessage, []byte(content))
}

func (r *RoomService) ConnectWebSocket(
	ctx context.Context,
	roomID string,
	opts ...option.RequestOption,
) (*RoomWebSocket, error) {
	opts = append(r.Options, opts...)

	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	cfg, err := requestconfig.NewRequestConfig(ctx, http.MethodGet, "", nil, nil, opts...)
	if err != nil {
		return nil, err
	}

	baseURL := cfg.BaseURL.String()

	// Convert http(s) to ws(s)
	wsURL := baseURL
	if after, ok := strings.CutPrefix(baseURL, "https://"); ok {
		wsURL = "wss://" + after
	} else if after0, ok0 := strings.CutPrefix(baseURL, "http://"); ok0 {
		wsURL = "ws://" + after0
	}

	path := fmt.Sprintf("%s/api/v1/rooms/%s/ws", wsURL, roomID)

	log.Printf("[WEBSOCKET_URL]: %s\n", path)

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
		return nil, fmt.Errorf("failed to connect to websocket: %w", err)
	}

	ws := &RoomWebSocket{
		conn:   conn,
		roomID: roomID,
	}

	return ws, nil
}
