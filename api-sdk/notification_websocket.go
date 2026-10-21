package apisdk

import (
	"context"
	"fmt"
	"slices"
	"sync"

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

// NotificationWebSocket is a connection to the per-user notification stream.
// It is safe for concurrent use.
type NotificationWebSocket struct {
	*wsConn
	userID         string
	mu             sync.RWMutex
	messageHandler func(NotificationWSMessage)
	errorHandler   func(error)
}

func (ws *NotificationWebSocket) SetMessageHandler(handler func(NotificationWSMessage)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.messageHandler = handler
}

// SetErrorHandler sets a callback invoked when the connection fails
// unexpectedly (not after Close or context cancellation).
func (ws *NotificationWebSocket) SetErrorHandler(handler func(error)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.errorHandler = handler
}

// Listen dispatches incoming notifications to the message handler until the
// connection fails, Close is called, or ctx is done; the connection is closed
// when it returns. It returns ctx.Err() if ctx ended the loop.
func (ws *NotificationWebSocket) Listen(ctx context.Context) error {
	return listen(ctx, ws.wsConn, func(msg NotificationWSMessage) {
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

func (c *Client) ConnectNotificationWebSocket(
	ctx context.Context,
	opts ...option.RequestOption,
) (*NotificationWebSocket, error) {
	opts = slices.Concat(c.Options, opts)

	conn, cfg, err := dialWebSocket(ctx, "api/v1/users/notifications/ws", opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to notification websocket: %w", err)
	}

	return &NotificationWebSocket{
		wsConn: newWSConn(conn),
		userID: cfg.Request.Header.Get("X-User-ID"),
	}, nil
}
