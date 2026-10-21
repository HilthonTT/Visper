package dependency

import (
	"github.com/hilthontt/visper/api/infrastructure/websocket"
)

func (c *Container) initWebSocket() {
	checkOrigin := websocket.NewOriginChecker(c.Config.Cors.AllowOrigins)

	c.WSRoomManager = websocket.NewRoomManager(checkOrigin)
	c.WSCore = websocket.NewCore(c.WSRoomManager, c.MessageRepo)
	c.NotificationCore = websocket.NewNotificationCore(checkOrigin)

	go c.WSCore.Run(c.ctx)
	go c.NotificationCore.Run(c.ctx)

	c.Logger.Info("WebSocket components initialized successfully")
}
