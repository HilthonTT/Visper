package file

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/application/usecases/file"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/storage"
	"github.com/hilthontt/visper/services/api/presentation/httpx"
)

// maxUploadRequestSize is the largest accepted upload request: the file limit
// plus room for the multipart envelope.
const maxUploadRequestSize = storage.MaxFileSize + 1<<20

type FilesController interface {
	Upload(ctx *gin.Context)
	GetRoomFiles(ctx *gin.Context)
	DeleteFile(ctx *gin.Context)
	// Down serves a stored file as a download.
	Down(ctx *gin.Context)
	// Proxy serves a stored file inline, with caching headers.
	Proxy(ctx *gin.Context)
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
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	// Bound the request body: the multipart parser would otherwise spool
	// arbitrarily large uploads to disk before the size check runs.
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxUploadRequestSize)

	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			httpx.Fail(ctx, http.StatusRequestEntityTooLarge, httpx.CodeTooLarge, storage.ErrFileTooLarge.Error())
			return
		}
		httpx.BadRequest(ctx, "file is required")
		return
	}

	uploaded, err := c.fileUseCase.UploadFile(ctx.Request.Context(), fileHeader, roomID, user.ID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, toFileResponse(uploaded, user.Username))
}

func (c *filesController) GetRoomFiles(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	files, err := c.fileUseCase.GetRoomFiles(ctx.Request.Context(), roomID, user.ID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	response := make([]FileResponse, len(files))
	for i, f := range files {
		response[i] = toFileResponse(f, "") // TODO: add usernames
	}
	ctx.JSON(http.StatusOK, response)
}

func (c *filesController) DeleteFile(ctx *gin.Context) {
	fileID, ok := httpx.RequireParam(ctx, "fileId", "file ID")
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	if err := c.fileUseCase.DeleteFile(ctx.Request.Context(), fileID, user.ID); err != nil {
		httpx.Error(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, httpx.SuccessResponse{Message: "file deleted successfully"})
}

func (c *filesController) Down(ctx *gin.Context) {
	_, fullPath, ok := c.resolveFile(ctx)
	if !ok {
		return
	}

	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.File(fullPath)
}

func (c *filesController) Proxy(ctx *gin.Context) {
	relativePath, fullPath, ok := c.resolveFile(ctx)
	if !ok {
		return
	}

	mimeType := storage.MIMETypeFromExtension(filepath.Ext(relativePath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	f, err := os.Open(fullPath)
	if err != nil {
		readError(ctx, err, "failed to open file")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		readError(ctx, err, "failed to stat file")
		return
	}

	// Stored file names are unique, so the name is a stable ETag.
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

// resolveFile maps the "*path" wildcard to a file inside the uploads
// directory. It writes the error response and returns ok=false on failure.
func (c *filesController) resolveFile(ctx *gin.Context) (relativePath, fullPath string, ok bool) {
	relativePath = strings.TrimPrefix(ctx.Param("path"), "/")
	if relativePath == "" {
		httpx.BadRequest(ctx, "file path is required")
		return "", "", false
	}

	fullPath, err := c.localStorage.GetFilePath(relativePath)
	if err != nil || !c.localStorage.FileExists(relativePath) {
		// Paths escaping the uploads directory are reported as not found.
		httpx.Fail(ctx, http.StatusNotFound, httpx.CodeNotFound, "file not found")
		return "", "", false
	}

	return relativePath, fullPath, true
}

func readError(ctx *gin.Context, err error, message string) {
	_ = ctx.Error(err)
	httpx.Fail(ctx, http.StatusInternalServerError, httpx.CodeInternal, message)
}

func toFileResponse(f *model.File, uploaderName string) FileResponse {
	return FileResponse{
		ID:        f.ID,
		Filename:  f.Filename,
		MimeType:  f.MimeType,
		Size:      f.Size,
		URL:       f.URL,
		CreatedAt: f.CreatedAt,
		Uploader:  UserResponse{ID: f.UserID, Username: uploaderName},
	}
}
