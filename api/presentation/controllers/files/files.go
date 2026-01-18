package files

import "github.com/gin-gonic/gin"

type FilesController interface {
	Proxy(c *gin.Context)
	Down(c *gin.Context)
}

type filesController struct{}

func NewFilesController() FilesController {
	return &filesController{}
}

func (r *filesController) Proxy(c *gin.Context) {
	// TODO: Implement handler
}

func (r *filesController) Down(c *gin.Context) {
	// TODO: Implement handler
}
