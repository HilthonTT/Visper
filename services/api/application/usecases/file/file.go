package file

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"

	"github.com/hilthontt/visper/services/api/domain/apperr"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/storage"
)

var (
	ErrRoomNotFound = apperr.New(apperr.NotFound, "room not found")
	ErrFileNotFound = apperr.New(apperr.NotFound, "file not found")
	ErrRoomExpired  = apperr.New(apperr.Gone, "room has expired")
	ErrNotMember    = apperr.New(apperr.Forbidden, "user is not a member of this room")
	ErrCannotDelete = apperr.New(apperr.Forbidden, "only the file uploader or room owner can delete files")
)

type FileUseCase interface {
	UploadFile(ctx context.Context, fileHeader *multipart.FileHeader, roomID, userID string) (*model.File, error)
	// GetRoomFiles lists a room's files with their uploaders' usernames.
	GetRoomFiles(ctx context.Context, roomID, userID string) ([]RoomFile, error)
	// DeleteFile deletes a file of roomID; only its uploader or the room's
	// owner may.
	DeleteFile(ctx context.Context, roomID, fileID, userID string) error
	CleanupOrphanedFiles(ctx context.Context) error
}

// RoomFile is a stored file and the username of whoever uploaded it ("" if
// they are no longer known).
type RoomFile struct {
	*model.File
	UploaderName string
}

type fileUseCase struct {
	fileRepo     repository.FileRepository
	roomRepo     repository.RoomRepository
	userRepo     repository.UserRepository
	localStorage *storage.LocalStorage
	serverURL    string
}

// NewFileUseCase returns the file use case. userRepo names uploaders who
// have left the room; it may be nil.
func NewFileUseCase(
	fileRepo repository.FileRepository,
	roomRepo repository.RoomRepository,
	userRepo repository.UserRepository,
	localStorage *storage.LocalStorage,
	serverURL string,
) FileUseCase {
	return &fileUseCase{
		fileRepo:     fileRepo,
		roomRepo:     roomRepo,
		userRepo:     userRepo,
		localStorage: localStorage,
		serverURL:    serverURL,
	}
}

func (uc *fileUseCase) UploadFile(ctx context.Context, fileHeader *multipart.FileHeader, roomID, userID string) (*model.File, error) {
	room, err := uc.getRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.HasExpired() {
		return nil, ErrRoomExpired
	}
	if !room.IsMember(userID) {
		return nil, ErrNotMember
	}

	relativePath, fileID, mimeType, err := uc.localStorage.SaveFile(fileHeader, roomID)
	if err != nil {
		return nil, storageError(err)
	}

	file := &model.File{
		ID:       fileID,
		RoomID:   roomID,
		UserID:   userID,
		Filename: fileHeader.Filename,
		// Use the type detected from the stored content, not the
		// client-supplied Content-Type header.
		MimeType: mimeType,
		Size:     fileHeader.Size,
		Path:     relativePath,
		URL:      fmt.Sprintf("%s/api/v1/d/%s", uc.serverURL, relativePath),
	}

	if err := uc.fileRepo.Create(ctx, file); err != nil {
		_ = uc.localStorage.DeleteFile(relativePath)
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}
	return file, nil
}

func (uc *fileUseCase) GetRoomFiles(ctx context.Context, roomID, userID string) ([]RoomFile, error) {
	room, err := uc.getRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	// Like messages and room details, the file list is for members only.
	if !room.IsMember(userID) {
		return nil, ErrNotMember
	}
	files, err := uc.fileRepo.GetByRoomID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	names := make(map[string]string, len(room.Members)+1)
	names[room.Owner.ID] = room.Owner.Username
	for _, m := range room.Members {
		names[m.ID] = m.Username
	}
	result := make([]RoomFile, len(files))
	for i, f := range files {
		name, ok := names[f.UserID]
		if !ok && uc.userRepo != nil {
			// The uploader left: their record may still be around. Look
			// each one up once; a missing record leaves the name empty.
			if u, err := uc.userRepo.GetByID(ctx, f.UserID); err == nil && u != nil {
				name = u.Username
			}
			names[f.UserID] = name
		}
		result[i] = RoomFile{File: f, UploaderName: name}
	}
	return result, nil
}

func (uc *fileUseCase) DeleteFile(ctx context.Context, roomID, fileID, userID string) error {
	file, err := uc.fileRepo.GetByID(ctx, fileID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && file == nil) {
		return ErrFileNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}
	// A file of another room is reported as missing, so the room in the
	// path can't be used to reach (or probe) other rooms' files.
	if file.RoomID != roomID {
		return ErrFileNotFound
	}

	room, err := uc.getRoom(ctx, file.RoomID)
	if err != nil {
		return err
	}
	if file.UserID != userID && room.Owner.ID != userID {
		return ErrCannotDelete
	}

	if err := uc.localStorage.DeleteFile(file.Path); err != nil {
		return fmt.Errorf("failed to delete file from storage: %w", err)
	}
	if err := uc.fileRepo.Delete(ctx, fileID); err != nil {
		return fmt.Errorf("failed to delete file metadata: %w", err)
	}
	return nil
}

func (uc *fileUseCase) CleanupOrphanedFiles(ctx context.Context) error {
	orphanedFiles, err := uc.fileRepo.GetOrphanedFiles(ctx)
	if err != nil {
		return fmt.Errorf("failed to get orphaned files: %w", err)
	}
	for _, file := range orphanedFiles {
		_ = uc.localStorage.DeleteFile(file.Path)
		_ = uc.fileRepo.Delete(ctx, file.ID)
	}

	roomDirs, err := uc.localStorage.GetAllRoomDirectories()
	if err != nil {
		return fmt.Errorf("failed to get room directories: %w", err)
	}
	for _, roomID := range roomDirs {
		// Only a definite "not found" means the room is gone; on any other
		// error (e.g. Redis unavailable) keep the files.
		if _, err := uc.roomRepo.GetByID(ctx, roomID); errors.Is(err, repository.ErrNotFound) {
			_ = uc.localStorage.DeleteRoomFiles(roomID)
		}
	}
	return nil
}

func (uc *fileUseCase) getRoom(ctx context.Context, roomID string) (*model.Room, error) {
	room, err := uc.roomRepo.GetByID(ctx, roomID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && room == nil) {
		return nil, ErrRoomNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}
	return room, nil
}

// storageError gives the storage errors the client can fix their kind.
func storageError(err error) error {
	switch {
	case errors.Is(err, storage.ErrFileTooLarge):
		return apperr.Wrap(apperr.TooLarge, err)
	case errors.Is(err, storage.ErrInvalidFileType), errors.Is(err, storage.ErrInvalidPath):
		return apperr.Wrap(apperr.Invalid, err)
	}
	return err
}
