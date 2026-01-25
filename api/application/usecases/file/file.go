package file

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/storage"
)

type FileUseCase interface {
	UploadFile(ctx context.Context, fileHeader *multipart.FileHeader, roomID, userID string) (*model.File, error)
	GetFile(ctx context.Context, fileID string) (*model.File, error)
	GetRoomFiles(ctx context.Context, roomID string) ([]*model.File, error)
	DeleteFile(ctx context.Context, fileID, userID string) error
	CleanupOrphanedFiles(ctx context.Context) error
}

type fileUseCase struct {
	fileRepo     repository.FileRepository
	roomRepo     repository.RoomRepository
	localStorage *storage.LocalStorage
	serverURL    string
}

func NewFileUseCase(
	fileRepo repository.FileRepository,
	roomRepo repository.RoomRepository,
	localStorage *storage.LocalStorage,
	serverURL string,
) FileUseCase {
	return &fileUseCase{
		fileRepo:     fileRepo,
		roomRepo:     roomRepo,
		localStorage: localStorage,
		serverURL:    serverURL,
	}
}

func (uc *fileUseCase) UploadFile(ctx context.Context, fileHeader *multipart.FileHeader, roomID, userID string) (*model.File, error) {
	room, err := uc.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("room not found")
	}

	if room.HasExpired() {
		return nil, fmt.Errorf("room has expired")
	}

	if !room.IsMember(userID) {
		return nil, fmt.Errorf("user is not a member of this room")
	}

	relativePath, fileID, err := uc.localStorage.SaveFile(fileHeader, roomID)
	if err != nil {
		return nil, err
	}

	file := &model.File{
		ID:       fileID,
		RoomID:   roomID,
		UserID:   userID,
		Filename: fileHeader.Filename,
		MimeType: fileHeader.Header.Get("Content-Type"),
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

func (uc *fileUseCase) GetFile(ctx context.Context, fileID string) (*model.File, error) {
	return uc.fileRepo.GetByID(ctx, fileID)
}

func (uc *fileUseCase) GetRoomFiles(ctx context.Context, roomID string) ([]*model.File, error) {
	_, err := uc.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("room not found")
	}

	return uc.fileRepo.GetByRoomID(ctx, roomID)
}

func (uc *fileUseCase) DeleteFile(ctx context.Context, fileID, userID string) error {
	file, err := uc.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found")
	}

	room, err := uc.roomRepo.GetByID(ctx, file.RoomID)
	if err != nil {
		return fmt.Errorf("room not found")
	}

	if file.UserID != userID && room.Owner.ID != userID {
		return fmt.Errorf("only the file uploader or room owner can delete files")
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
		// Delete from local storage
		_ = uc.localStorage.DeleteFile(file.Path)

		// Delete metadata
		_ = uc.fileRepo.Delete(ctx, file.ID)
	}

	roomDirs, err := uc.localStorage.GetAllRoomDirectories()
	if err != nil {
		return fmt.Errorf("failed to get room directories: %w", err)
	}

	for _, roomID := range roomDirs {
		_, err := uc.roomRepo.GetByID(ctx, roomID)
		if err != nil {
			// Room doesn't exist, delete the directory
			_ = uc.localStorage.DeleteRoomFiles(roomID)
		}
	}

	return nil
}
