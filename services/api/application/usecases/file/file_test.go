package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/storage"
)

type memFileRepo struct {
	files map[string]*model.File
}

func (r *memFileRepo) Create(_ context.Context, f *model.File) error { r.files[f.ID] = f; return nil }
func (r *memFileRepo) GetByID(_ context.Context, id string) (*model.File, error) {
	if f, ok := r.files[id]; ok {
		return f, nil
	}
	return nil, repository.ErrNotFound
}
func (r *memFileRepo) GetByRoomID(_ context.Context, roomID string) ([]*model.File, error) {
	var out []*model.File
	for _, f := range r.files {
		if f.RoomID == roomID {
			out = append(out, f)
		}
	}
	return out, nil
}
func (r *memFileRepo) Delete(_ context.Context, id string) error               { delete(r.files, id); return nil }
func (r *memFileRepo) GetOrphanedFiles(context.Context) ([]*model.File, error) { return nil, nil }

type memRoomRepo struct {
	repository.RoomRepository
	rooms map[string]*model.Room
}

func (r *memRoomRepo) GetByID(_ context.Context, id string) (*model.Room, error) {
	if room, ok := r.rooms[id]; ok {
		return room, nil
	}
	return nil, repository.ErrNotFound
}

type memUserRepo struct {
	repository.UserRepository
	users   map[string]*model.User
	lookups int
}

func (r *memUserRepo) GetByID(_ context.Context, id string) (*model.User, error) {
	r.lookups++
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}

func newTestUseCase(t *testing.T) (*fileUseCase, *memFileRepo, *memUserRepo, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := storage.NewLocalStorageAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := &memFileRepo{files: map[string]*model.File{}}
	rooms := &memRoomRepo{rooms: map[string]*model.Room{
		"r1": {
			ID: "r1", Owner: model.User{ID: "owner", Username: "Olive"},
			Members:   []model.User{{ID: "owner", Username: "Olive"}, {ID: "ann", Username: "Ann"}, {ID: "bo", Username: "Bo"}},
			CreatedAt: time.Now(), Expiry: time.Hour,
		},
		"r2": {ID: "r2", Owner: model.User{ID: "other"}, CreatedAt: time.Now(), Expiry: time.Hour},
	}}
	users := &memUserRepo{users: map[string]*model.User{"gone": {ID: "gone", Username: "Gus"}}}
	uc := NewFileUseCase(files, rooms, users, st, "http://x").(*fileUseCase)
	return uc, files, users, dir
}

// addFile stores a file on disk and in the repo.
func addFile(t *testing.T, repo *memFileRepo, dir, id, roomID, userID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, roomID), 0o750); err != nil {
		t.Fatal(err)
	}
	rel := roomID + "/" + id + ".enc"
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo.files[id] = &model.File{ID: id, RoomID: roomID, UserID: userID, Path: rel}
}

func TestGetRoomFilesNamesUploaders(t *testing.T) {
	uc, repo, users, dir := newTestUseCase(t)
	addFile(t, repo, dir, "f1", "r1", "ann")
	addFile(t, repo, dir, "f2", "r1", "gone")
	addFile(t, repo, dir, "f3", "r1", "gone")
	addFile(t, repo, dir, "f4", "r1", "unknown")

	files, err := uc.GetRoomFiles(context.Background(), "r1", "bo")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.ID] = f.UploaderName
	}
	want := map[string]string{"f1": "Ann", "f2": "Gus", "f3": "Gus", "f4": ""}
	for id, name := range want {
		if got[id] != name {
			t.Errorf("uploader of %s = %q, want %q", id, got[id], name)
		}
	}
	// Departed uploaders are looked up once each.
	if users.lookups != 2 {
		t.Errorf("user lookups = %d, want 2", users.lookups)
	}

	if _, err := uc.GetRoomFiles(context.Background(), "r1", "stranger"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("non-member: %v", err)
	}
}

func TestDeleteFile(t *testing.T) {
	uc, repo, _, dir := newTestUseCase(t)
	ctx := context.Background()
	addFile(t, repo, dir, "f1", "r1", "ann")
	addFile(t, repo, dir, "f2", "r1", "ann")
	addFile(t, repo, dir, "other", "r2", "other")

	if err := uc.DeleteFile(ctx, "r1", "f1", "bo"); !errors.Is(err, ErrCannotDelete) {
		t.Fatalf("another member deleting: %v", err)
	}
	// The room in the path must be the file's room.
	if err := uc.DeleteFile(ctx, "r1", "other", "owner"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("file of another room: %v", err)
	}
	if err := uc.DeleteFile(ctx, "r1", "missing", "ann"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	if err := uc.DeleteFile(ctx, "r1", "f1", "ann"); err != nil {
		t.Fatalf("uploader deleting: %v", err)
	}
	if err := uc.DeleteFile(ctx, "r1", "f2", "owner"); err != nil {
		t.Fatalf("owner deleting: %v", err)
	}
	if len(repo.files) != 1 {
		t.Fatalf("files left = %d, want 1", len(repo.files))
	}
	if _, err := os.Stat(filepath.Join(dir, "r1", "f1.enc")); !os.IsNotExist(err) {
		t.Fatalf("file still on disk: %v", err)
	}
}
