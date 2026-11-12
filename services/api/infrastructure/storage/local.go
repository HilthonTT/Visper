package storage

import (
	"bytes"
	"errors"
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

var (
	ErrFileTooLarge    = errors.New("file size exceeds maximum allowed size of 5MB")
	ErrInvalidFileType = errors.New("invalid file: uploads must be encrypted on the client")
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

// EncryptedFileMagic starts every file a client encrypted for a room (see
// the SDK's EncryptFile). The server cannot decrypt uploads and only stores
// ones carrying it, so a client that forgot to encrypt is refused instead of
// leaking the file.
var EncryptedFileMagic = []byte("VISPER\x00\x01")

// EncryptedFileExt is the extension of stored files; they are opaque.
const EncryptedFileExt = ".enc"

// minEncryptedFileSize is the size of an encrypted empty file: the magic, a
// 24-byte nonce and a 16-byte authentication tag.
var minEncryptedFileSize = len(EncryptedFileMagic) + 24 + 16

// EncryptedFileMIME is the MIME type of stored files.
const EncryptedFileMIME = "application/octet-stream"

// SaveFile validates and stores an uploaded encrypted file. It returns the
// path of the stored file relative to the uploads directory (slash
// separated), the file ID and the MIME type. The uploaded file name is
// ignored: clients keep real names inside their encrypted messages.
func (s *LocalStorage) SaveFile(file *multipart.FileHeader, roomID string) (relativePath, fileID, mimeType string, err error) {
	if file.Size > MaxFileSize {
		return "", "", "", ErrFileTooLarge
	}

	if !isSingleSegment(roomID) {
		return "", "", "", ErrInvalidPath
	}

	src, err := file.Open()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	head := make([]byte, minEncryptedFileSize)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", "", "", fmt.Errorf("failed to read uploaded file: %w", err)
	}
	head = head[:n]
	if n < minEncryptedFileSize || !bytes.HasPrefix(head, EncryptedFileMagic) {
		return "", "", "", ErrInvalidFileType
	}
	mimeType = EncryptedFileMIME
	ext := EncryptedFileExt

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

// MIMETypeFromExtension returns the MIME type served for a stored file's
// extension (including the dot), or "" if it is unknown. Image types remain
// for files uploaded before uploads were encrypted.
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
	case EncryptedFileExt:
		return EncryptedFileMIME
	default:
		return ""
	}
}
