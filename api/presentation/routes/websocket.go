package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/presentation/controllers/websocket"
)

func WebsocketRoutes(router *gin.RouterGroup, controller websocket.WebSocketController) {
	rooms := router.Group("/rooms")
	{
		rooms.GET("/:id/ws", controller.HandleConnection)
	}
}
