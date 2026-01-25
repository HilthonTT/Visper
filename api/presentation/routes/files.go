package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/hilthontt/visper/api/presentation/controllers/file"
	"github.com/hilthontt/visper/api/presentation/middlewares"
)

func FilesRoute(router *gin.RouterGroup, controller file.FilesController, logger *logger.Logger) {
	router.GET("/d/*path", controller.Down)
	router.GET("/p/*path", controller.Proxy)
	router.HEAD("/d/*path", controller.Down)
	router.HEAD("/p/*path", controller.Proxy)

	filesGroup := router.Group("/rooms/:id/files")
	filesGroup.Use(middlewares.RateLimiterMiddleware(cache.GetRedis(), logger, middlewares.StrictRateLimiterConfig()))
	{
		filesGroup.POST("/upload", controller.Upload)
		filesGroup.GET("", controller.GetRoomFiles)
		filesGroup.DELETE("/:fileId", controller.DeleteFile)
	}
}
