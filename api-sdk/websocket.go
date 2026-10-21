package apisdk

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hilthontt/visper/api-sdk/option"
)

// ErrWebSocketClosed is returned when using a websocket after Close.
var ErrWebSocketClosed = errors.New("websocket connection is closed")

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
	Encrypted bool   `json:"encrypted"`
}

type MessageUpdatedPayload struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	RoomID    string `json:"roomId"`
	Timestamp string `json:"timestamp"`
	Encrypted bool   `json:"encrypted"`
}

type MessageDeletedPayload struct {
	ID        string `json:"id"`
	RoomID    string `json:"roomId"`
	Timestamp string `json:"timestamp"`
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

// BootPayload is the payload of a [Kicked] event.
type BootPayload struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
}

type RoomDeletedPayload struct {
	RoomID string `json:"roomid"`
}

type RoomUpdatedPayload struct {
	RoomID   string `json:"roomId"`
	JoinCode string `json:"joinCode"`
}

// RoomWebSocket is a connection to a room's event stream. It is safe for
// concurrent use.
type RoomWebSocket struct {
	*wsConn
	roomID         string
	mu             sync.RWMutex
	messageHandler func(WSMessage)
	errorHandler   func(error)
}

func (ws *RoomWebSocket) SetMessageHandler(handler func(WSMessage)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.messageHandler = handler
}

// SetErrorHandler sets a callback invoked when the connection fails
// unexpectedly (not after Close or context cancellation).
func (ws *RoomWebSocket) SetErrorHandler(handler func(error)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.errorHandler = handler
}

// Listen dispatches incoming messages to the message handler until the
// connection fails, Close is called, or ctx is done; the connection is closed
// when it returns. It returns ctx.Err() if ctx ended the loop.
func (ws *RoomWebSocket) Listen(ctx context.Context) error {
	return listen(ctx, ws.wsConn, func(msg WSMessage) {
		ws.mu.RLock()
		handler := ws.messageHandler
		ws.mu.RUnlock()
		if handler != nil {
			handler(msg)
		}
	}, func(err error) {
		ws.mu.RLock()
		handler := ws.errorHandler
		ws.mu.RUnlock()
		if handler != nil {
			handler(err)
		}
	})
}

// SendMessage sends content as a text frame.
func (ws *RoomWebSocket) SendMessage(content string) error {
	return ws.write(websocket.TextMessage, []byte(content))
}

func (r *RoomService) ConnectWebSocket(
	ctx context.Context,
	roomID string,
	opts ...option.RequestOption,
) (*RoomWebSocket, error) {
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}
	opts = slices.Concat(r.Options, opts)

	conn, _, err := dialWebSocket(ctx, fmt.Sprintf("api/v1/rooms/%s/ws", url.PathEscape(roomID)), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket: %w", err)
	}

	return &RoomWebSocket{
		wsConn: newWSConn(conn),
		roomID: roomID,
	}, nil
}
