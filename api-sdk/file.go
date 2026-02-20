package apisdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
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

type FileResponse struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Uploader  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"uploader"`
}

func (r *FileResponse) UnmarshalJSON(data []byte) error {
	// Use a plain alias to avoid recursion
	type plain FileResponse
	return json.Unmarshal(data, (*plain)(r))
}

func (s *FileService) Upload(ctx context.Context, roomID, filePath, userID string, opts ...option.RequestOption) (*FileResponse, error) {
	opts = slices.Concat(s.Options, opts)

	// Build the config to get base URL and HTTP client
	cfg, err := requestconfig.NewRequestConfig(ctx, http.MethodPost, "", nil, nil, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build request config: %w", err)
	}

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
	writer.Close()

	url := fmt.Sprintf("%s/api/v1/rooms/%s/files/upload", cfg.BaseURL, roomID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result FileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
