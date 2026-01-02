package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/presentation/controllers/message"
)

func MessageRoutes(router *gin.RouterGroup, controller message.MessageController) {
	router.POST("/rooms/:id/messages", controller.SendMessage)
	router.GET("/rooms/:id/messages", controller.GetMessages)
	router.GET("/rooms/:id/messages/after", controller.GetMessagesAfter)
	router.GET("/rooms/:id/messages/count", controller.GetMessageCount)
}
