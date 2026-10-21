package file

import (
	"errors"
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

// maxUploadRequestSize is the largest accepted upload request: the file limit
// plus room for the multipart envelope.
const maxUploadRequestSize = storage.MaxFileSize + 1<<20

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

	// Bound the request body: the multipart parser would otherwise spool
	// arbitrarily large uploads to disk before the size check runs.
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxUploadRequestSize)

	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			ctx.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
				Error:   "file_too_large",
				Message: storage.ErrFileTooLarge.Error(),
			})
			return
		}
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

		switch {
		case err.Error() == "room not found":
			status = http.StatusNotFound
			errorCode = "not_found"
		case err.Error() == "room has expired":
			status = http.StatusGone
			errorCode = "expired"
		case err.Error() == "user is not a member of this room":
			status = http.StatusForbidden
			errorCode = "forbidden"
		case errors.Is(err, storage.ErrFileTooLarge):
			status = http.StatusRequestEntityTooLarge
			errorCode = "file_too_large"
		case errors.Is(err, storage.ErrInvalidFileType), errors.Is(err, storage.ErrInvalidPath):
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

// resolveFile maps the "*path" wildcard to a file inside the uploads
// directory. It writes the error response and returns ok=false on failure.
func (c *filesController) resolveFile(ctx *gin.Context) (relativePath, fullPath string, ok bool) {
	relativePath = strings.TrimPrefix(ctx.Param("path"), "/")
	if relativePath == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "file path is required",
		})
		return "", "", false
	}

	fullPath, err := c.localStorage.GetFilePath(relativePath)
	if err != nil || !c.localStorage.FileExists(relativePath) {
		// Paths escaping the uploads directory are reported as not found.
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "file not found",
		})
		return "", "", false
	}

	return relativePath, fullPath, true
}

func (c *filesController) Proxy(ctx *gin.Context) {
	filePath, fullPath, ok := c.resolveFile(ctx)
	if !ok {
		return
	}

	mimeType := storage.MIMETypeFromExtension(filepath.Ext(filePath))
	if mimeType == "" {
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

	filename := filepath.Base(fullPath)
	etag := fmt.Sprintf(`"%s"`, filename)

	ctx.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Header("Cache-Control", "public, max-age=31536000, immutable")
	ctx.Header("ETag", etag)

	if ctx.GetHeader("If-None-Match") == etag {
		ctx.Status(http.StatusNotModified)
		return
	}

	ctx.DataFromReader(http.StatusOK, info.Size(), mimeType, f, nil)
}

func (c *filesController) Down(ctx *gin.Context) {
	_, fullPath, ok := c.resolveFile(ctx)
	if !ok {
		return
	}

	ctx.Header("X-Content-Type-Options", "nosniff")
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
