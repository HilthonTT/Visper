package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestDistributedCache(t *testing.T) (*DistributedCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	dc := NewDistributedCache(client, "test:", Options{})
	t.Cleanup(dc.Close)
	return dc, mr
}

func TestSAddIf(t *testing.T) {
	ctx := context.Background()
	dc, mr := newTestDistributedCache(t)

	type room struct {
		JoinCode   string `json:"joinCode"`
		SecureCode string `json:"secureCode"`
	}
	if err := dc.Set(ctx, "room:r1", room{JoinCode: "ABC123", SecureCode: "secret"}, 0); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		record string
		member string
		want   map[string]string
		result int
	}{
		{"missing record", "room:gone", "u1", nil, SAddRecordMissing},
		{"no conditions", "room:r1", "owner", nil, SAddAdded},
		{"matching join code", "room:r1", "u2", map[string]string{"joinCode": "ABC123"}, SAddAdded},
		{"both codes match", "room:r1", "u3", map[string]string{"joinCode": "ABC123", "secureCode": "secret"}, SAddAdded},
		{"stale join code", "room:r1", "kicked", map[string]string{"joinCode": "OLD000"}, SAddFieldMismatch},
		{"stale secure code", "room:r1", "kicked", map[string]string{"joinCode": "ABC123", "secureCode": "old"}, SAddFieldMismatch},
	}
	for _, c := range cases {
		got, err := dc.SAddIf(ctx, c.record, "room:r1:users", c.member, c.want)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.result {
			t.Errorf("%s: got %d, want %d", c.name, got, c.result)
		}
	}

	members, err := mr.Members("test:room:r1:users")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 {
		t.Fatalf("members = %v, want owner, u2 and u3", members)
	}
	for _, m := range members {
		if m == "kicked" {
			t.Fatal("a join with stale codes was added")
		}
	}
	if mr.Exists("test:room:gone:users") {
		t.Fatal("a join to a missing room created its member set")
	}
}

func TestSAddIfSeesRotationThroughLocalCache(t *testing.T) {
	ctx := context.Background()
	dc, mr := newTestDistributedCache(t)

	// This instance cached the old codes; another instance then rotated them
	// in Redis. The check must use Redis, not the stale local copy.
	if err := dc.Set(ctx, "room:r1", map[string]string{"joinCode": "OLD000"}, 0); err != nil {
		t.Fatal(err)
	}
	if err := mr.Set("test:room:r1", `{"joinCode":"NEW111"}`); err != nil {
		t.Fatal(err)
	}

	got, err := dc.SAddIf(ctx, "room:r1", "room:r1:users", "kicked", map[string]string{"joinCode": "OLD000"})
	if err != nil {
		t.Fatal(err)
	}
	if got != SAddFieldMismatch {
		t.Fatalf("got %d, want SAddFieldMismatch", got)
	}
}
