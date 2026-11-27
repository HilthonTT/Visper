package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/cache"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace/noop"
)

func newTestRoomRepository(t *testing.T) (*roomRepository, *cache.DistributedCache) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	dc := cache.NewDistributedCache(client, "test:", cache.Options{})
	t.Cleanup(dc.Close)

	tracer := noop.NewTracerProvider().Tracer("test")
	users := NewUserRepository(dc, tracer)
	return NewRoomRepository(dc, users, tracer).(*roomRepository), dc
}

func assertJoinCode(t *testing.T, r *roomRepository, code, wantRoomID string) {
	t.Helper()
	room, err := r.GetByJoinCode(context.Background(), code)
	if wantRoomID == "" {
		if !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("GetByJoinCode(%q) = %v, %v; want ErrNotFound", code, room, err)
		}
		return
	}
	if err != nil || room.ID != wantRoomID {
		t.Fatalf("GetByJoinCode(%q) = %v, %v; want room %s", code, room, err, wantRoomID)
	}
}

func TestJoinCodeIndexFollowsTheRoom(t *testing.T) {
	r, _ := newTestRoomRepository(t)
	ctx := context.Background()

	room := &model.Room{ID: "r1", JoinCode: "ABC123", Owner: model.User{ID: "owner"}}
	if err := r.Create(ctx, room); err != nil {
		t.Fatal(err)
	}
	assertJoinCode(t, r, "ABC123", "r1")

	room.JoinCode = "XYZ789"
	if err := r.Update(ctx, room); err != nil {
		t.Fatal(err)
	}
	assertJoinCode(t, r, "ABC123", "")
	assertJoinCode(t, r, "XYZ789", "r1")

	if err := r.Delete(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	assertJoinCode(t, r, "XYZ789", "")
}

// Two rooms drawing the same code must not hide each other.
func TestJoinCodeIndexKeepsCollidingRooms(t *testing.T) {
	r, _ := newTestRoomRepository(t)
	ctx := context.Background()

	for _, id := range []string{"r1", "r2"} {
		if err := r.Create(ctx, &model.Room{ID: id, JoinCode: "SAME00"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Delete(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	assertJoinCode(t, r, "SAME00", "r2")
}

// Rooms stored before the index existed are added by IndexJoinCodes.
func TestIndexJoinCodesBackfills(t *testing.T) {
	r, dc := newTestRoomRepository(t)
	ctx := context.Background()

	if err := dc.Set(ctx, "room:old", &model.Room{ID: "old", JoinCode: "OLD123"}, 0); err != nil {
		t.Fatal(err)
	}
	if err := dc.SAdd(ctx, "rooms", "old"); err != nil {
		t.Fatal(err)
	}
	assertJoinCode(t, r, "OLD123", "")

	if err := r.IndexJoinCodes(ctx); err != nil {
		t.Fatal(err)
	}
	assertJoinCode(t, r, "OLD123", "old")
}
