package apisdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hilthontt/visper/sdk/go/option"
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
	// MessageReactions carries a message's full, new list of reactions
	// (MessageReactionsPayload).
	MessageReactions = "message.reactions"

	// PresenceUpdate lists the members connected right now (PresencePayload).
	PresenceUpdate = "presence.update"
	// MemberTyping is sent when a member types (TypingPayload). It is not
	// repeated more than every two seconds; treat it as stale after a few.
	MemberTyping = "member.typing"
	// ReadUpdate is the last message a member has read (ReadPayload). On
	// connect, every member's current marker follows the message history.
	ReadUpdate = "read.update"

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
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	UserID    string            `json:"userId"`
	Username  string            `json:"username"`
	Timestamp string            `json:"timestamp"`
	Encrypted bool              `json:"encrypted"`
	ReplyToID string            `json:"replyToId,omitempty"`
	Reactions []ReactionPayload `json:"reactions,omitempty"`
}

type ReactionPayload struct {
	UserID    string `json:"userId"`
	Emoji     string `json:"emoji"`
	Encrypted bool   `json:"encrypted"`
}

type MessageReactionsPayload struct {
	ID        string            `json:"id"`
	Reactions []ReactionPayload `json:"reactions"`
}

type PresencePayload struct {
	Online []string `json:"online"`
}

type TypingPayload struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

type ReadPayload struct {
	UserID    string `json:"userId"`
	MessageID string `json:"messageId"`
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

// Messages are sent with MessageService.Send (so they are stored and
// encrypted); the socket only carries these control frames.

// SendTyping tells the room the user is typing. The server relays at most one
// every two seconds per connection.
func (ws *RoomWebSocket) SendTyping() error {
	return ws.sendFrame(map[string]string{"type": "typing"})
}

// SendRead tells the room the user has read up to messageID.
func (ws *RoomWebSocket) SendRead(messageID string) error {
	if messageID == "" {
		return ErrMissingIDParameter
	}
	return ws.sendFrame(map[string]string{"type": "read", "messageId": messageID})
}

func (ws *RoomWebSocket) sendFrame(frame map[string]string) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return ws.write(websocket.TextMessage, data)
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
