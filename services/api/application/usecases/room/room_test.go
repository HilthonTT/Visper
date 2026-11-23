package room

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"go.uber.org/zap"
)

// fakeRoomRepo mimics the Redis repository.
type fakeRoomRepo struct {
	rooms   map[string]*model.Room
	removed []string
	deleted []string
	added   []string
	// ops records RemoveUser ("remove") and Update ("update") calls in order.
	ops       []string
	updateErr error
	addErr    error
	addCodes  []repository.RoomCodes
}

func (f *fakeRoomRepo) Create(context.Context, *model.Room) error { return nil }
func (f *fakeRoomRepo) GetByID(_ context.Context, id string) (*model.Room, error) {
	if r, ok := f.rooms[id]; ok {
		return r, nil
	}
	return nil, repository.ErrNotFound
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
func (f *fakeRoomRepo) AddUser(_ context.Context, _ string, u model.User, codes repository.RoomCodes) error {
	f.addCodes = append(f.addCodes, codes)
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, u.ID)
	return nil
}
func (f *fakeRoomRepo) RemoveUser(_ context.Context, _ string, userID string) error {
	f.ops = append(f.ops, "remove")
	f.removed = append(f.removed, userID)
	return nil
}
func (f *fakeRoomRepo) GetUsers(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeRoomRepo) Update(context.Context, *model.Room) error {
	f.ops = append(f.ops, "update")
	return f.updateErr
}

func newTestUseCase(repo *fakeRoomRepo) RoomUseCase {
	return NewRoomUseCase(repo, nil, &logger.Logger{Log: zap.NewNop()}, nil)
}

func TestMissingRoomIsReportedAsNotFound(t *testing.T) {
	uc := newTestUseCase(&fakeRoomRepo{rooms: map[string]*model.Room{}})
	ctx := context.Background()

	checks := map[string]error{
		"Delete":               uc.Delete(ctx, "missing", "u1"),
		"LeaveRoom":            uc.LeaveRoom(ctx, "missing", "u1"),
		"KickMember":           func() error { _, err := uc.KickMember(ctx, "missing", "u2", "u1"); return err }(),
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
	var notified []string
	uc := NewRoomUseCase(repo, nil, &logger.Logger{Log: zap.NewNop()}, func(id string) {
		notified = append(notified, id)
	})

	n, err := uc.ExpireRooms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(repo.deleted) != 1 || repo.deleted[0] != "expired" {
		t.Fatalf("expired %d, deleted %v; want only \"expired\"", n, repo.deleted)
	}
	if len(notified) != 1 || notified[0] != "expired" {
		t.Fatalf("onExpired got %v; want [expired]", notified)
	}
}

// A room found expired on read must also be reported, so that its websocket
// clients are disconnected.
func TestLazyExpiryNotifies(t *testing.T) {
	repo := &fakeRoomRepo{rooms: map[string]*model.Room{
		"r1": {ID: "r1", CreatedAt: time.Now().Add(-2 * time.Hour), Expiry: time.Hour},
	}}
	var notified []string
	uc := NewRoomUseCase(repo, nil, &logger.Logger{Log: zap.NewNop()}, func(id string) {
		notified = append(notified, id)
	})

	if _, err := uc.GetByID(context.Background(), "r1"); err == nil || err.Error() != "room has expired" {
		t.Fatalf("GetByID: got %v, want \"room has expired\"", err)
	}
	if len(notified) != 1 || notified[0] != "r1" {
		t.Fatalf("onExpired got %v; want [r1]", notified)
	}
}

func TestKickRotatesCodes(t *testing.T) {
	owner := model.User{ID: "owner"}
	repo := &fakeRoomRepo{rooms: map[string]*model.Room{
		"r1": {ID: "r1", Owner: owner, JoinCode: "ABC123", SecureCode: "secret",
			Members: []model.User{owner, {ID: "member"}}},
	}}
	uc := newTestUseCase(repo)

	room, err := uc.KickMember(context.Background(), "r1", "member", "owner")
	if err != nil {
		t.Fatalf("kick: %v", err)
	}
	// The kicked member saw the old codes, so they must not work any more.
	if room.JoinCode == "ABC123" || room.SecureCode == "secret" {
		t.Fatalf("codes not rotated: %q %q", room.JoinCode, room.SecureCode)
	}
	if room.IsMember("member") {
		t.Fatal("kicked member still listed")
	}
}

func TestOwnerActionsRefuseExpiredRooms(t *testing.T) {
	owner := model.User{ID: "owner"}
	repo := &fakeRoomRepo{rooms: map[string]*model.Room{
		"r1": {ID: "r1", Owner: owner, CreatedAt: time.Now().Add(-2 * time.Hour), Expiry: time.Hour,
			Members: []model.User{owner, {ID: "member"}}},
	}}
	uc := newTestUseCase(repo)

	if _, err := uc.GenerateNewJoinCode(context.Background(), "owner", "r1"); !errors.Is(err, ErrRoomExpired) {
		t.Fatalf("new join code on expired room: got %v, want ErrRoomExpired", err)
	}
}

func TestKickRotatesCodesBeforeRemovingTheMember(t *testing.T) {
	owner := model.User{ID: "owner"}
	newRepo := func() *fakeRoomRepo {
		return &fakeRoomRepo{rooms: map[string]*model.Room{
			"r1": {ID: "r1", Owner: owner, JoinCode: "ABC123", SecureCode: "secret",
				Members: []model.User{owner, {ID: "member"}}},
		}}
	}

	repo := newRepo()
	if _, err := newTestUseCase(repo).KickMember(context.Background(), "r1", "member", "owner"); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if want := []string{"update", "remove"}; !slices.Equal(repo.ops, want) {
		t.Fatalf("ops = %v, want %v", repo.ops, want)
	}

	// If the codes can't be rotated, the member must still be in the room,
	// so the owner can retry the kick.
	repo = newRepo()
	repo.updateErr = errors.New("redis down")
	if _, err := newTestUseCase(repo).KickMember(context.Background(), "r1", "member", "owner"); err == nil {
		t.Fatal("kick succeeded although the codes weren't rotated")
	}
	if len(repo.removed) != 0 {
		t.Fatalf("member removed although the kick failed: %v", repo.removed)
	}
}

func TestJoinPassesTheCodesAndRefusesRotatedOnes(t *testing.T) {
	owner := model.User{ID: "owner"}
	repo := &fakeRoomRepo{rooms: map[string]*model.Room{
		"r1": {ID: "r1", Owner: owner, JoinCode: "ABC123", SecureCode: "secret", Members: []model.User{owner}},
	}}
	uc := newTestUseCase(repo)
	joiner := model.User{ID: "joiner"}

	if err := uc.JoinRoomWithCodes(context.Background(), "r1", "ABC123", "secret", joiner); err != nil {
		t.Fatalf("join: %v", err)
	}
	want := repository.RoomCodes{JoinCode: "ABC123", SecureCode: "secret"}
	if len(repo.addCodes) != 1 || repo.addCodes[0] != want {
		t.Fatalf("AddUser got codes %v, want %v", repo.addCodes, want)
	}

	// A kick rotated the codes between reading the room and joining it.
	repo.addErr = repository.ErrCodesChanged
	err := uc.JoinRoom(context.Background(), "r1", model.User{ID: "kicked"}, repository.RoomCodes{JoinCode: "ABC123"})
	if !errors.Is(err, ErrInvalidCodes) {
		t.Fatalf("join with rotated codes: got %v, want ErrInvalidCodes", err)
	}
}
