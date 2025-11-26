package ws

import (
	"sync"

	"github.com/gorilla/websocket"
)

type connWrapper struct {
	conn  *websocket.Conn
	mutex sync.Mutex
}

func newConnWrapper(c *websocket.Conn) *connWrapper {
	return &connWrapper{conn: c}
}

func (w *connWrapper) WriteJSON(v any) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.conn.WriteJSON(v)
}

func (w *connWrapper) Close() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.conn.Close()
}
