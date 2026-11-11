package room

import (
	"context"
	"testing"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// fakeRoomRepo mimics the Redis repository: a missing room is redis.Nil.
type fakeRoomRepo struct {
	rooms   map[string]*model.Room
	removed []string
	deleted []string
	added   []string
}

func (f *fakeRoomRepo) Create(context.Context, *model.Room) error { return nil }
func (f *fakeRoomRepo) GetByID(_ context.Context, id string) (*model.Room, error) {
	if r, ok := f.rooms[id]; ok {
		return r, nil
	}
	return nil, redis.Nil
}
func (f *fakeRoomRepo) GetAll(context.Context) ([]*model.Room, error) {
	out := make([]*model.Room, 0, len(f.rooms))
	for _, r := range f.rooms {
		out = append(out, r)
	}
	return out, nil
}
func (f *fakeRoomRepo) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	delete(f.rooms, id)
	return nil
}
func (f *fakeRoomRepo) AddUser(_ context.Context, _ string, u model.User) error {
	f.added = append(f.added, u.ID)
	return nil
}
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

func TestJoinRoomWithCodesRejectsWrongCodes(t *testing.T) {
	repo := &fakeRoomRepo{rooms: map[string]*model.Room{
		"r1": {ID: "r1", JoinCode: "ABC123", SecureCode: "secret"},
	}}
	uc := newTestUseCase(repo)
	ctx := context.Background()
	stranger := model.User{ID: "stranger"}

	cases := map[string][2]string{
		"no codes":         {"", ""},
		"wrong join code":  {"XXXXXX", "secret"},
		"wrong secure":     {"ABC123", "guess"},
		"secure as prefix": {"ABC123", "secre"},
	}
	for name, c := range cases {
		err := uc.JoinRoomWithCodes(ctx, "r1", c[0], c[1], stranger)
		if err == nil || err.Error() != "invalid join code or secure token" {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if len(repo.added) != 0 {
		t.Fatalf("no one may be added with wrong codes, got %v", repo.added)
	}

	if err := uc.JoinRoomWithCodes(ctx, "missing", "ABC123", "secret", stranger); err == nil || err.Error() != "room not found" {
		t.Fatalf("missing room: got %v", err)
	}
}

func TestExpireRoomsDeletesOnlyExpiredRooms(t *testing.T) {
	now := time.Now()
	repo := &fakeRoomRepo{rooms: map[string]*model.Room{
		"expired":   {ID: "expired", CreatedAt: now.Add(-2 * time.Hour), Expiry: time.Hour},
		"live":      {ID: "live", CreatedAt: now, Expiry: time.Hour},
		"no-expiry": {ID: "no-expiry", CreatedAt: now.Add(-48 * time.Hour)},
	}}
	uc := newTestUseCase(repo)

	n, err := uc.ExpireRooms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(repo.deleted) != 1 || repo.deleted[0] != "expired" {
		t.Fatalf("expired %d, deleted %v; want only \"expired\"", n, repo.deleted)
	}
}
