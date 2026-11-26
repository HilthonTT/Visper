package ports

import (
	"errors"
	"mime/multipart"
)

// Errors a FileStorage returns for requests the client can fix.
var (
	ErrFileTooLarge    = errors.New("file size exceeds maximum allowed size of 5MB")
	ErrInvalidFileType = errors.New("invalid file: uploads must be encrypted on the client")
	// ErrInvalidPath is returned for paths that are absolute or escape the
	// storage root (e.g. "../config.yml").
	ErrInvalidPath = errors.New("invalid file path")
)

// FileStorage stores uploaded files, grouped by room. Paths are relative to
// the storage root.
type FileStorage interface {
	SaveFile(file *multipart.FileHeader, roomID string) (relativePath, fileID, mimeType string, err error)
	DeleteFile(relativePath string) error
	DeleteRoomFiles(roomID string) error
	GetAllRoomDirectories() ([]string, error)
}
