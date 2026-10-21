package apisdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

type FileService struct {
	Options []option.RequestOption
}

func NewFileService(opts ...option.RequestOption) *FileService {
	return &FileService{
		Options: opts,
	}
}

type FileUploader struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type FileResponse struct {
	ID        string       `json:"id"`
	Filename  string       `json:"filename"`
	MimeType  string       `json:"mimetype"`
	Size      int64        `json:"size"`
	URL       string       `json:"url"`
	CreatedAt time.Time    `json:"createdAt"`
	Uploader  FileUploader `json:"uploader"`
}

func (r *FileResponse) UnmarshalJSON(data []byte) error {
	// Use a plain alias to avoid recursion
	type plain FileResponse
	return json.Unmarshal(data, (*plain)(r))
}

// Upload uploads the file at filePath to the room. If userID is non-empty it
// is sent as the X-User-ID header.
func (s *FileService) Upload(ctx context.Context, roomID, filePath, userID string, opts ...option.RequestOption) (*FileResponse, error) {
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}
	opts = slices.Concat(s.Options, opts)

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("failed to write file to form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize form: %w", err)
	}

	if userID != "" {
		opts = append(opts, option.WithHeader("X-User-ID", userID))
	}
	// Retrying would require re-reading the body; uploads are not idempotent.
	opts = append(opts,
		option.WithRequestBody(writer.FormDataContentType(), body.Bytes()),
		option.WithMaxRetries(0),
	)

	path := fmt.Sprintf("api/v1/rooms/%s/files/upload", url.PathEscape(roomID))
	res := &FileResponse{}
	if err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, nil, res, opts...); err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	return res, nil
}
