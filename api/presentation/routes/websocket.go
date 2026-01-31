package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/presentation/controllers/websocket"
)

func WebsocketRoutes(
	router *gin.RouterGroup,
	controller websocket.WebSocketController,
	notificationController websocket.UserNotificationController,
) {
	rooms := router.Group("/rooms")
	{
		rooms.GET("/:id/ws", controller.HandleConnection)
	}

	users := router.Group("/users")
	{
		users.GET("/notifications/ws", notificationController.HandleUserNotificationConnection)
		users.POST("/notifications/self-room-invite", notificationController.NotifySelfRoomInvite)
	}
}
