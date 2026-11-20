package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIdentityStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", IdentityFile)
	s, err := OpenIdentityStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := s.Identity(); ok {
		t.Fatal("a new store has an identity")
	}
	if err := s.SetIdentity("tok", "sign", "dev"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.SaveRoom(StoredRoom{ID: "r1", OwnerID: "o", Keys: []string{"k1", "k2"}, Epoch: 1, Verified: map[string]string{"bob": "fp"}, SavedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRoom(StoredRoom{ID: "r2", Keys: []string{"k"}, SavedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("identity file permissions = %o, want 600", perm)
		}
	}

	again, err := OpenIdentityStore(path)
	if err != nil {
		t.Fatal(err)
	}
	tok, sign, dev, ok := again.Identity()
	if !ok || tok != "tok" || sign != "sign" || dev != "dev" {
		t.Fatalf("identity = %q %q %q %v", tok, sign, dev, ok)
	}
	rooms := again.Rooms()
	if len(rooms) != 2 || rooms[0].ID != "r2" || rooms[1].Verified["bob"] != "fp" || rooms[1].Epoch != 1 {
		t.Fatalf("rooms = %+v", rooms)
	}

	if err := again.RemoveRoom("r2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Room("r2"); ok {
		t.Fatal("r2 still stored")
	}
	// A new identity forgets the old one's rooms.
	if err := again.SetIdentity("tok2", "s", "d"); err != nil {
		t.Fatal(err)
	}
	if len(again.Rooms()) != 0 {
		t.Fatal("rooms survived a new identity")
	}
}

func TestIdentityStoreRemoveExpired(t *testing.T) {
	s, _ := OpenIdentityStore(filepath.Join(t.TempDir(), IdentityFile))
	now := time.Now()
	_ = s.SaveRoom(StoredRoom{ID: "old", ExpiresAt: now.Add(-time.Minute)})
	_ = s.SaveRoom(StoredRoom{ID: "live", ExpiresAt: now.Add(time.Hour)})
	_ = s.SaveRoom(StoredRoom{ID: "forever"})
	if err := s.RemoveExpired(now); err != nil {
		t.Fatal(err)
	}
	if len(s.Rooms()) != 2 {
		t.Fatalf("rooms = %+v", s.Rooms())
	}
	if _, ok := s.Room("old"); ok {
		t.Fatal("expired room kept")
	}
}

func TestCorruptIdentityIsMovedAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, IdentityFile)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenIdentityStore(path)
	if err == nil || !strings.Contains(err.Error(), ".bak") {
		t.Fatalf("err = %v, want a note about the backup", err)
	}
	if _, _, _, ok := s.Identity(); ok {
		t.Fatal("corrupt file gave an identity")
	}
	if b, err := os.ReadFile(path + ".bak"); err != nil || string(b) != "{not json" {
		t.Fatalf("backup = %q, %v", b, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("corrupt file still in place")
	}

	if err := ForgetIdentity(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatal("backup not removed by ForgetIdentity")
	}
	if err := ForgetIdentity(path); err != nil {
		t.Fatalf("forgetting twice: %v", err)
	}
}
