package apisdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/hilthontt/visper/sdk/go/internal/requestconfig"
	"github.com/hilthontt/visper/sdk/go/option"
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

// MaxUploadSize is the largest file the API stores, after encryption.
const MaxUploadSize = 5 << 20

// ErrFileTooLarge is returned for files that would exceed MaxUploadSize once
// encrypted.
var ErrFileTooLarge = fmt.Errorf("file is larger than %d bytes", MaxUploadSize-encryptedFileOverhead)

// UploadEncrypted encrypts the file at filePath with the room key keyB64 and
// uploads it to the room. The API only stores the ciphertext and never sees
// the file's name: the caller shares the returned URL (and any name) in an
// encrypted message, and readers decrypt the download with DecryptFile. If
// userToken is non-empty it identifies the uploader (see WithUserToken).
func (s *FileService) UploadEncrypted(ctx context.Context, roomID, filePath, keyB64, userToken string, opts ...option.RequestOption) (*FileResponse, error) {
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}
	opts = slices.Concat(s.Options, opts)

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	if info.Size() > MaxUploadSize-encryptedFileOverhead {
		return nil, ErrFileTooLarge
	}
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	ciphertext, err := EncryptFile(plaintext, keyB64)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// A fixed name: the real one would leak to the server.
	part, err := writer.CreateFormFile("file", "file.enc")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(ciphertext); err != nil {
		return nil, fmt.Errorf("failed to write file to form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize form: %w", err)
	}

	if userToken != "" {
		opts = append(opts, option.WithUserToken(userToken))
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
