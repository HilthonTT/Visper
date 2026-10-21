package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	MaxFileSize     = 5 * 1024 * 1024 // 5MBs
	UploadsBasePath = "./uploads"
)

var (
	ErrFileTooLarge    = errors.New("file size exceeds maximum allowed size of 5MB")
	ErrInvalidFileType = errors.New("invalid file type, only images are allowed")
	// ErrInvalidPath is returned for paths that are absolute or escape the
	// uploads directory (e.g. "../config.yml").
	ErrInvalidPath = errors.New("invalid file path")
)

type LocalStorage struct {
	basePath string
}

func NewLocalStorage() (*LocalStorage, error) {
	return NewLocalStorageAt(UploadsBasePath)
}

// NewLocalStorageAt creates a storage rooted at basePath.
func NewLocalStorageAt(basePath string) (*LocalStorage, error) {
	storage := &LocalStorage{
		basePath: basePath,
	}

	if err := os.MkdirAll(storage.basePath, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create uploads directory: %w", err)
	}

	return storage, nil
}

// resolve maps a slash-separated path relative to the uploads directory to a
// filesystem path, rejecting anything that would escape the directory.
func (s *LocalStorage) resolve(relativePath string) (string, error) {
	rel := filepath.FromSlash(strings.TrimPrefix(relativePath, "/"))
	if rel == "" || !filepath.IsLocal(rel) {
		return "", ErrInvalidPath
	}
	return filepath.Join(s.basePath, rel), nil
}

// isSingleSegment reports whether name is a plain directory/file name.
func isSingleSegment(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, `/\`) && filepath.IsLocal(name)
}

// SaveFile validates and stores an uploaded image. It returns the path of the
// stored file relative to the uploads directory (slash separated), the file
// ID and the MIME type detected from the file content.
func (s *LocalStorage) SaveFile(file *multipart.FileHeader, roomID string) (relativePath, fileID, mimeType string, err error) {
	if file.Size > MaxFileSize {
		return "", "", "", ErrFileTooLarge
	}

	if !isSingleSegment(roomID) {
		return "", "", "", ErrInvalidPath
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	expectedType := extensionToMIME(ext)
	if expectedType == "" {
		return "", "", "", ErrInvalidFileType
	}

	src, err := file.Open()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Check the actual content, not just the extension, so that e.g. HTML
	// cannot be uploaded as "image.png".
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", "", "", fmt.Errorf("failed to read uploaded file: %w", err)
	}
	head = head[:n]

	mimeType = http.DetectContentType(head)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", "", "", ErrInvalidFileType
	}

	fileID = uuid.NewString()
	filename := fileID + ext

	roomPath := filepath.Join(s.basePath, roomID)
	if err := os.MkdirAll(roomPath, 0o750); err != nil {
		return "", "", "", fmt.Errorf("failed to create room directory: %w", err)
	}

	filePath := filepath.Join(roomPath, filename)

	dst, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create destination file: %w", err)
	}

	// Never write more than MaxFileSize, whatever the multipart header claimed.
	content := io.MultiReader(bytes.NewReader(head), src)
	written, copyErr := io.Copy(dst, io.LimitReader(content, MaxFileSize+1))
	closeErr := dst.Close()

	switch {
	case copyErr != nil:
		err = fmt.Errorf("failed to save file: %w", copyErr)
	case closeErr != nil:
		err = fmt.Errorf("failed to save file: %w", closeErr)
	case written > MaxFileSize:
		err = ErrFileTooLarge
	}
	if err != nil {
		_ = os.Remove(filePath)
		return "", "", "", err
	}

	return roomID + "/" + filename, fileID, mimeType, nil
}

func (s *LocalStorage) DeleteFile(relativePath string) error {
	fullPath, err := s.resolve(relativePath)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalStorage) DeleteRoomFiles(roomID string) error {
	if !isSingleSegment(roomID) {
		return ErrInvalidPath
	}

	return os.RemoveAll(filepath.Join(s.basePath, roomID))
}

// FileExists reports whether relativePath is a regular file inside the
// uploads directory.
func (s *LocalStorage) FileExists(relativePath string) bool {
	fullPath, err := s.resolve(relativePath)
	if err != nil {
		return false
	}

	info, err := os.Stat(fullPath)
	return err == nil && info.Mode().IsRegular()
}

func (s *LocalStorage) GetAllRoomDirectories() ([]string, error) {
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, err
	}

	roomDirs := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			roomDirs = append(roomDirs, entry.Name())
		}
	}

	return roomDirs, nil
}

// GetFilePath returns the filesystem path for relativePath, or
// ErrInvalidPath if it would escape the uploads directory.
func (s *LocalStorage) GetFilePath(relativePath string) (string, error) {
	return s.resolve(relativePath)
}

// MIMETypeFromExtension returns the image MIME type for a file extension
// (including the dot), or "" if the extension is not an allowed image type.
func MIMETypeFromExtension(ext string) string {
	return extensionToMIME(strings.ToLower(ext))
}

func extensionToMIME(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return ""
	}
}
