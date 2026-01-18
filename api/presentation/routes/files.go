package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/infrastructure/stream"
	"github.com/hilthontt/visper/api/presentation/controllers/files"
	"github.com/hilthontt/visper/api/presentation/middlewares"
)

func FilesRoute(router *gin.RouterGroup, controller files.FilesController) {
	downloadLimiter := middlewares.DownloadRateLimiter(stream.ClientDownloadLimit)

	router.GET("/d/*path", downloadLimiter, controller.Down)
	router.GET("/p/*path", downloadLimiter, controller.Proxy)

	router.HEAD("/d/*path", controller.Down)
	router.HEAD("/p/*path", controller.Proxy)
}
