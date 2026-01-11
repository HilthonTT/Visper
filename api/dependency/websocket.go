package dependency

import (
	"context"

	"github.com/hilthontt/visper/api/infrastructure/websocket"
)

func (c *Container) initWebSocket() {
	c.WSRoomManager = websocket.NewRoomManager()
	c.WSCore = websocket.NewCore(c.RoomRepo, c.MessageRepo)

	c.ctx, c.cancel = context.WithCancel(context.Background())

	go c.WSCore.Run(c.ctx)

	c.Logger.Info("WebSocket components initialized successfully")
}
