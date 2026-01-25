package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	MaxFileSize     = 5 * 1024 * 1024 // 5MBs
	UploadsBasePath = "./uploads"
)

type LocalStorage struct {
	basePath string
}

func NewLocalStorage() (*LocalStorage, error) {
	storage := &LocalStorage{
		basePath: UploadsBasePath,
	}

	if err := os.MkdirAll(storage.basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create uploads directory: %w", err)
	}

	return storage, nil
}

func (s *LocalStorage) SaveFile(file *multipart.FileHeader, roomID string) (string, string, error) {
	if file.Size > MaxFileSize {
		return "", "", fmt.Errorf("file size exceeds maximum allowed size of 5MB")
	}

	if !s.isValidImageType(file.Header.Get("Content-Type")) {
		return "", "", fmt.Errorf("")
	}

	src, err := file.Open()
	if err != nil {
		return "", "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	ext := filepath.Ext(file.Filename)
	fileID := uuid.NewString()
	filename := fileID + ext

	roomPath := filepath.Join(s.basePath, roomID)
	if err := os.MkdirAll(roomPath, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create room directory: %w", err)
	}

	filePath := filepath.Join(roomPath, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", "", fmt.Errorf("failed to save file: %w", err)
	}

	relativePath := filepath.Join(roomID, filename)

	return relativePath, fileID, nil
}

func (s *LocalStorage) DeleteFile(filePath string) error {
	fullPath := filepath.Join(s.basePath, filePath)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil
	}

	return os.Remove(fullPath)
}

func (s *LocalStorage) DeleteRoomFiles(roomID string) error {
	roomPath := filepath.Join(s.basePath, roomID)

	if _, err := os.Stat(roomPath); os.IsNotExist(err) {
		return nil
	}

	return os.RemoveAll(roomPath)
}

func (s *LocalStorage) FileExists(relativePath string) bool {
	fullPath := filepath.Join(s.basePath, relativePath)
	_, err := os.Stat(fullPath)
	return err == nil
}

func (s *LocalStorage) isValidImageType(contentType string) bool {
	validTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}

	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return validTypes[contentType]
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

func (s *LocalStorage) GetFilePath(relativePath string) string {
	return filepath.Join(s.basePath, relativePath)
}
