package room

import (
	"context"
	"testing"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// fakeRoomRepo mimics the Redis repository: a missing room is redis.Nil.
type fakeRoomRepo struct {
	rooms   map[string]*model.Room
	removed []string
}

func (f *fakeRoomRepo) Create(context.Context, *model.Room) error { return nil }
func (f *fakeRoomRepo) GetByID(_ context.Context, id string) (*model.Room, error) {
	if r, ok := f.rooms[id]; ok {
		return r, nil
	}
	return nil, redis.Nil
}
func (f *fakeRoomRepo) GetAll(context.Context) ([]*model.Room, error)     { return nil, nil }
func (f *fakeRoomRepo) Delete(context.Context, string) error              { return nil }
func (f *fakeRoomRepo) AddUser(context.Context, string, model.User) error { return nil }
func (f *fakeRoomRepo) RemoveUser(_ context.Context, _ string, userID string) error {
	f.removed = append(f.removed, userID)
	return nil
}
func (f *fakeRoomRepo) GetUsers(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeRoomRepo) Update(context.Context, *model.Room) error          { return nil }

func newTestUseCase(repo *fakeRoomRepo) RoomUseCase {
	return NewRoomUseCase(repo, nil, &logger.Logger{Log: zap.NewNop()})
}

func TestMissingRoomIsReportedAsNotFound(t *testing.T) {
	uc := newTestUseCase(&fakeRoomRepo{rooms: map[string]*model.Room{}})
	ctx := context.Background()

	checks := map[string]error{
		"Delete":               uc.Delete(ctx, "missing", "u1"),
		"LeaveRoom":            uc.LeaveRoom(ctx, "missing", "u1"),
		"KickMember":           uc.KickMember(ctx, "missing", "u2", "u1"),
		"GenerateNewJoinCode":  func() error { _, err := uc.GenerateNewJoinCode(ctx, "u1", "missing"); return err }(),
		"RegenerateSecureCode": func() error { _, err := uc.RegenerateSecureCode(ctx, "u1", "missing"); return err }(),
	}
	for name, err := range checks {
		if err == nil || err.Error() != "room not found" {
			t.Errorf("%s: got %v, want \"room not found\"", name, err)
		}
	}
}

func TestLeaveRoomRequiresMembership(t *testing.T) {
	owner := model.User{ID: "owner"}
	repo := &fakeRoomRepo{rooms: map[string]*model.Room{
		"r1": {ID: "r1", Owner: owner, Members: []model.User{owner, {ID: "member"}}},
	}}
	uc := newTestUseCase(repo)

	err := uc.LeaveRoom(context.Background(), "r1", "stranger")
	if err == nil || err.Error() != "user is not a member of this room" {
		t.Fatalf("stranger leaving: got %v", err)
	}
	if len(repo.removed) != 0 {
		t.Fatalf("stranger must not be removed, got %v", repo.removed)
	}

	if err := uc.LeaveRoom(context.Background(), "r1", "member"); err != nil {
		t.Fatalf("member leaving: %v", err)
	}
}
