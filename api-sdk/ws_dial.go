package apisdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

// websocketHandshakeHeaders lists headers that gorilla/websocket sets itself
// and rejects if supplied by the caller.
var websocketHandshakeHeaders = map[string]bool{
	"Upgrade":                  true,
	"Connection":               true,
	"Sec-Websocket-Key":        true,
	"Sec-Websocket-Version":    true,
	"Sec-Websocket-Extensions": true,
	"Sec-Websocket-Protocol":   true,
}

// dialWebSocket resolves path against the configured base URL (switching the
// scheme to ws/wss), forwards the configured headers and cookies, and dials.
func dialWebSocket(ctx context.Context, path string, opts ...option.RequestOption) (*websocket.Conn, *requestconfig.RequestConfig, error) {
	cfg, err := requestconfig.NewRequestConfig(ctx, http.MethodGet, "", nil, nil, opts...)
	if err != nil {
		return nil, nil, err
	}

	base := cfg.BaseURL
	if base == nil {
		base = cfg.DefaultBaseURL
	}
	if base == nil {
		return nil, nil, fmt.Errorf("base URL is not configured")
	}

	u, err := base.Parse(strings.TrimLeft(path, "/"))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid websocket path: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}

	headers := http.Header{}
	for key, values := range cfg.Request.Header {
		if websocketHandshakeHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		for _, value := range values {
			headers.Add(key, value)
		}
	}

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}
	if cfg.HTTPClient != nil && cfg.HTTPClient.Jar != nil {
		// gorilla/websocket looks cookies up under the equivalent http(s) URL.
		dialer.Jar = cfg.HTTPClient.Jar
	}

	conn, resp, err := dialer.DialContext(ctx, u.String(), headers)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, nil, err
	}
	return conn, cfg, nil
}

// wsConn wraps a websocket connection with the locking gorilla/websocket
// requires (one concurrent writer) and context-aware shutdown.
type wsConn struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
	closed    chan struct{}
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{conn: conn, closed: make(chan struct{})}
}

func (c *wsConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.writeMu.Lock()
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
		c.writeMu.Unlock()
		c.closeErr = c.conn.Close()
	})
	return c.closeErr
}

func (c *wsConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *wsConn) write(messageType int, data []byte) error {
	if c.isClosed() {
		return ErrWebSocketClosed
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}

// listen reads JSON messages into a new T and passes them to handle until the
// connection fails, Close is called, or ctx is done. It closes the connection
// before returning. A nil error is never returned; ctx.Err() is returned when
// the context ended the loop and ErrWebSocketClosed after Close.
func listen[T any](ctx context.Context, c *wsConn, handle func(T), onError func(error)) error {
	defer c.Close()

	// ReadJSON blocks, so closing the connection is the only way to unblock it
	// when the context is cancelled.
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()

	for {
		var msg T
		if err := c.conn.ReadJSON(&msg); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if c.isClosed() {
				return ErrWebSocketClosed
			}
			if onError != nil {
				onError(err)
			}
			return fmt.Errorf("websocket read error: %w", err)
		}
		handle(msg)
	}
}
