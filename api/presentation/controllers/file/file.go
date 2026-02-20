package file

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/application/usecases/file"
	"github.com/hilthontt/visper/api/infrastructure/storage"
	"github.com/hilthontt/visper/api/presentation/middlewares"
)

type FilesController interface {
	Upload(ctx *gin.Context)
	Down(ctx *gin.Context)
	Proxy(ctx *gin.Context)
	GetRoomFiles(ctx *gin.Context)
	DeleteFile(ctx *gin.Context)
}

type filesController struct {
	fileUseCase  file.FileUseCase
	localStorage *storage.LocalStorage
}

func NewFilesController(fileUseCase file.FileUseCase, localStorage *storage.LocalStorage) FilesController {
	return &filesController{
		fileUseCase:  fileUseCase,
		localStorage: localStorage,
	}
}

func (c *filesController) Upload(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "file is required",
		})
		return
	}

	file, err := c.fileUseCase.UploadFile(ctx.Request.Context(), fileHeader, roomID, user.ID)
	if err != nil {
		status := http.StatusInternalServerError
		errorCode := "upload_failed"

		switch err.Error() {
		case "room not found":
			status = http.StatusNotFound
			errorCode = "not_found"
		case "room has expired":
			status = http.StatusGone
			errorCode = "expired"
		case "user is not a member of this room":
			status = http.StatusForbidden
			errorCode = "forbidden"
		case "file size exceeds maximum allowed size of 5MB":
			status = http.StatusRequestEntityTooLarge
			errorCode = "file_too_large"
		case "invalid file type, only images are allowed":
			status = http.StatusBadRequest
			errorCode = "invalid_file_type"
		}

		ctx.JSON(status, ErrorResponse{
			Error:   errorCode,
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, FileResponse{
		ID:        file.ID,
		Filename:  file.Filename,
		MimeType:  file.MimeType,
		Size:      file.Size,
		URL:       file.URL,
		CreatedAt: file.CreatedAt,
		Uploader: UserResponse{
			ID:       user.ID,
			Username: user.Username,
		},
	})
}

func (c *filesController) Proxy(ctx *gin.Context) {
	filePath := ctx.Param("path")
	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "file path is required",
		})
		return
	}

	// Strip leading slash added by Gin's wildcard
	if len(filePath) > 0 && filePath[0] == '/' {
		filePath = filePath[1:]
	}

	if !c.localStorage.FileExists(filePath) {
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "file not found",
		})
		return
	}

	fullPath := c.localStorage.GetFilePath(filePath)

	ext := strings.ToLower(filepath.Ext(filePath))
	mimeTypes := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".bmp":  "image/bmp",
	}

	mimeType, ok := mimeTypes[ext]
	if !ok {
		mimeType = "application/octet-stream"
	}

	f, err := os.Open(fullPath)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "read_error",
			Message: "failed to open file",
		})
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "read_error",
			Message: "failed to stat file",
		})
		return
	}

	filename := filepath.Base(filePath)
	ctx.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	ctx.Header("Content-Type", mimeType)
	ctx.Header("Content-Length", fmt.Sprintf("%d", info.Size()))

	ctx.Header("Cache-Control", "public, max-age=31536000, immutable")
	ctx.Header("ETag", fmt.Sprintf(`"%s"`, filename))

	if match := ctx.GetHeader("If-None-Match"); match != "" {
		if match == fmt.Sprintf(`"%s"`, filename) {
			ctx.Status(http.StatusNotModified)
			return
		}
	}

	ctx.DataFromReader(http.StatusOK, info.Size(), mimeType, f, nil)
}

func (c *filesController) Down(ctx *gin.Context) {
	filePath := ctx.Param("path")
	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "file path is required",
		})
		return
	}

	if len(filePath) > 0 && filePath[0] == '/' {
		filePath = filePath[1:]
	}

	if !c.localStorage.FileExists(filePath) {
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "file not found",
		})
		return
	}

	fullPath := c.localStorage.GetFilePath(filePath)

	ctx.File(fullPath)
}

func (c *filesController) DeleteFile(ctx *gin.Context) {
	fileID := ctx.Param("fileId")
	if fileID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "file ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	if err := c.fileUseCase.DeleteFile(ctx.Request.Context(), fileID, user.ID); err != nil {
		status := http.StatusInternalServerError
		errorCode := "delete_failed"

		switch err.Error() {
		case "file not found":
			status = http.StatusNotFound
			errorCode = "not_found"
		case "room not found":
			status = http.StatusNotFound
			errorCode = "not_found"
		case "only the file uploader or room owner can delete files":
			status = http.StatusForbidden
			errorCode = "forbidden"
		}

		ctx.JSON(status, ErrorResponse{
			Error:   errorCode,
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "file deleted successfully",
	})
}

func (c *filesController) GetRoomFiles(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	_, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	files, err := c.fileUseCase.GetRoomFiles(ctx.Request.Context(), roomID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "room not found" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
		})
		return
	}

	response := make([]FileResponse, len(files))
	for i, file := range files {
		response[i] = FileResponse{
			ID:        file.ID,
			Filename:  file.Filename,
			MimeType:  file.MimeType,
			Size:      file.Size,
			URL:       file.URL,
			CreatedAt: file.CreatedAt,
			Uploader: UserResponse{
				ID:       file.UserID,
				Username: "", // TODO: add usernames
			},
		}
	}

	ctx.JSON(http.StatusOK, response)
}
