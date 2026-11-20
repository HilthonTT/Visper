package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// IdentityFile is the name of the file, in ConfigDir, that keeps the CLI's
// identity and the rooms it is in across runs.
const IdentityFile = "identity.json"

// identityVersion is the format of the identity file.
const identityVersion = 1

// StoredIdentity is the content of the identity file. Everything in it is
// secret: the user token is the CLI's credential with the API, the keys
// sign its messages and open new room keys, and the room keys decrypt the
// rooms. The file is written with owner-only (0600) permissions.
type StoredIdentity struct {
	Version int `json:"version"`
	// UserToken is the private credential the API derives the user ID from.
	UserToken string `json:"userToken"`
	// SigningKey is the Ed25519 seed (base64url) signing what we send.
	SigningKey string `json:"signingKey"`
	// DeviceKey is the X25519 private key (base64url) new room keys are
	// sealed to.
	DeviceKey string `json:"deviceKey"`
	// Rooms are the rooms we are in, by room ID.
	Rooms map[string]StoredRoom `json:"rooms,omitempty"`
}

// StoredRoom is what reopening a room without an invite needs.
type StoredRoom struct {
	ID       string `json:"id"`
	JoinCode string `json:"joinCode"`
	// OwnerID is the owner pinned when the room was first entered: only
	// their rekeys are accepted, and a server claiming another owner later
	// is not believed.
	OwnerID   string `json:"ownerId"`
	OwnerName string `json:"ownerName,omitempty"`
	// ExpiresAt is when the room expires (zero if never).
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
	// Keys are the room keys, oldest first; the last is current.
	Keys []string `json:"keys"`
	// Epoch is the rekey epoch of the current key.
	Epoch int `json:"epoch"`
	// Verified maps verified members to their pinned key fingerprint.
	Verified map[string]string `json:"verified,omitempty"`
	// SavedAt is when this entry last changed.
	SavedAt time.Time `json:"savedAt"`
}

// Expired reports whether the room has expired at now.
func (r StoredRoom) Expired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt)
}

// IdentityStore reads and writes the identity file. It is safe for
// concurrent use; every change is written to disk at once.
type IdentityStore struct {
	path string
	mu   sync.Mutex
	data StoredIdentity
}

// IdentityPath returns where the identity file is kept.
func IdentityPath() string {
	return filepath.Join(ConfigDir(), IdentityFile)
}

// OpenIdentityStore loads the identity file at path. A missing file gives
// an empty store. A file that can't be parsed is moved aside to path+".bak"
// (so the keys in it aren't silently lost) and an empty store is returned
// with an error saying so; the caller should log it and carry on.
func OpenIdentityStore(path string) (*IdentityStore, error) {
	s := &IdentityStore{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("can't read %s: %w", path, err)
	}

	var data StoredIdentity
	if err := json.Unmarshal(raw, &data); err != nil || data.Version != identityVersion {
		reason := "unsupported version"
		if err != nil {
			reason = err.Error()
		}
		backup := path + ".bak"
		if renameErr := os.Rename(path, backup); renameErr != nil {
			return s, fmt.Errorf("%s is unreadable (%s) and couldn't be moved aside: %w", path, reason, renameErr)
		}
		return s, fmt.Errorf("%s was unreadable (%s); moved it to %s and started a new identity", path, reason, backup)
	}
	s.data = data
	return s, nil
}

// MoveAside renames the identity file to path+".bak" and empties the store,
// for a file that parsed but holds unusable keys.
func (s *IdentityStore) MoveAside() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Rename(s.path, s.path+".bak"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("couldn't move %s aside: %w", s.path, err)
	}
	s.data = StoredIdentity{}
	return nil
}

// Path returns the identity file's path.
func (s *IdentityStore) Path() string { return s.path }

// Identity returns the stored credential and keys; ok is false if the store
// has none yet.
func (s *IdentityStore) Identity() (userToken, signingKey, deviceKey string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.data
	return d.UserToken, d.SigningKey, d.DeviceKey, d.UserToken != "" && d.SigningKey != "" && d.DeviceKey != ""
}

// SetIdentity replaces the stored credential and keys, forgetting every
// room (their memberships belong to the old identity), and saves.
func (s *IdentityStore) SetIdentity(userToken, signingKey, deviceKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = StoredIdentity{
		Version:    identityVersion,
		UserToken:  userToken,
		SigningKey: signingKey,
		DeviceKey:  deviceKey,
	}
	return s.saveLocked()
}

// Rooms returns the stored rooms, most recently saved first.
func (s *IdentityStore) Rooms() []StoredRoom {
	s.mu.Lock()
	defer s.mu.Unlock()
	rooms := make([]StoredRoom, 0, len(s.data.Rooms))
	for _, r := range s.data.Rooms {
		rooms = append(rooms, r)
	}
	slices.SortFunc(rooms, func(a, b StoredRoom) int {
		if c := b.SavedAt.Compare(a.SavedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return rooms
}

// Room returns the stored room roomID.
func (s *IdentityStore) Room(roomID string) (StoredRoom, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data.Rooms[roomID]
	return r, ok
}

// SaveRoom stores (or replaces) a room and saves.
func (s *IdentityStore) SaveRoom(r StoredRoom) error {
	if r.ID == "" {
		return errors.New("room ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Rooms == nil {
		s.data.Rooms = map[string]StoredRoom{}
	}
	if r.SavedAt.IsZero() {
		r.SavedAt = time.Now()
	}
	s.data.Rooms[r.ID] = r
	return s.saveLocked()
}

// RemoveRoom forgets a room and saves. Forgetting a room that isn't stored
// does nothing.
func (s *IdentityStore) RemoveRoom(roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Rooms[roomID]; !ok {
		return nil
	}
	delete(s.data.Rooms, roomID)
	return s.saveLocked()
}

// RemoveExpired forgets the rooms that expired by now and saves if any did.
func (s *IdentityStore) RemoveExpired(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := false
	for id, r := range s.data.Rooms {
		if r.Expired(now) {
			delete(s.data.Rooms, id)
			removed = true
		}
	}
	if !removed {
		return nil
	}
	return s.saveLocked()
}

func (s *IdentityStore) saveLocked() error {
	if s.data.Version == 0 {
		s.data.Version = identityVersion
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode identity: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := writeFileAtomic(s.path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", s.path, err)
	}
	return nil
}

// ForgetIdentity deletes the identity file at path (and a backup of an
// unreadable one), so the next run starts with a new identity. Rooms
// owned by the old identity stay on the server until they expire.
func ForgetIdentity(path string) error {
	var errs []error
	for _, p := range []string{path, path + ".bak"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
