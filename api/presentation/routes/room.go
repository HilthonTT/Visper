package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/presentation/controllers/room"
)

func RoomRoutes(router *gin.RouterGroup, controller room.RoomController) {
	rooms := router.Group("/rooms")
	{
		rooms.POST("", controller.CreateRoom)
		rooms.GET("/:id", controller.GetRoom)
		rooms.DELETE("/:id", controller.DeleteRoom)
		rooms.PUT("/:id/join-code", controller.GenerateNewJoinCode)

		rooms.POST("/join-code", controller.JoinRoomByJoinCode)

		rooms.POST("/:id/join", controller.JoinRoom)
		rooms.POST("/:id/leave", controller.LeaveRoom)
		rooms.GET("/:id/membership", controller.CheckMembership)
		rooms.POST("/:id/membership/:userId", controller.KickMember)
	}
}
