package repository

import (
	"context"
	"sync"
	"time"

	"github.com/hilthontt/visper/internal/domain"
)

type roomRepository struct {
	rooms          map[string]*domain.Room // ID -> Room
	joinCodeIndex  map[string]*domain.Room // JoinCode -> Room
	lastAccess     map[string]time.Time    // ID -> last access time
	capacity       uint
	idleRoomExpiry time.Duration
	mu             *sync.RWMutex
}

func NewRoomRepository(capacity uint, idleRoomExpiry time.Duration) domain.RoomRepository {
	if capacity == 0 {
		capacity = 100
	}
	if idleRoomExpiry == 0 {
		idleRoomExpiry = 30 * time.Minute
	}

	return &roomRepository{
		rooms:          make(map[string]*domain.Room),
		joinCodeIndex:  make(map[string]*domain.Room),
		lastAccess:     make(map[string]time.Time),
		capacity:       capacity,
		idleRoomExpiry: idleRoomExpiry,
		mu:             &sync.RWMutex{},
	}
}

func (r *roomRepository) touch(roomID string) {
	r.lastAccess[roomID] = time.Now()
}

func (r *roomRepository) evictIdle() {
	cutoff := time.Now().Add(-r.idleRoomExpiry)
	for id, last := range r.lastAccess {
		if last.Before(cutoff) {
			if room, exists := r.rooms[id]; exists {
				delete(r.joinCodeIndex, room.JoinCode)
			}
			delete(r.rooms, id)
			delete(r.lastAccess, id)
		}
	}
}

// enforceCapacity ensures we don't exceed capacity by removing oldest-accessed rooms.
func (r *roomRepository) enforceCapacity() {
	if uint(len(r.rooms)) <= r.capacity {
		return
	}

	// Sort by last access (oldest first)
	type entry struct {
		id   string
		time time.Time
	}
	var entries []entry
	for id, t := range r.lastAccess {
		entries = append(entries, entry{id, t})
	}
	// Simple selection of oldest (no need for full sort if we just need to drop a few)
	for i := 0; i < len(entries)-int(r.capacity); i++ {
		oldest := entries[i]
		if room, exists := r.rooms[oldest.id]; exists {
			delete(r.joinCodeIndex, room.JoinCode)
		}
		delete(r.rooms, oldest.id)
		delete(r.lastAccess, oldest.id)
	}
}

// Create adds a room if ID and JoinCode are unique and capacity allows.
func (r *roomRepository) Create(ctx context.Context, room *domain.Room) error {
	if room == nil || room.ID == "" || room.JoinCode == "" {
		return domain.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Clean up idle rooms first
	r.evictIdle()

	// Check existence
	if _, exists := r.rooms[room.ID]; exists {
		return domain.ErrRoomAlreadyExists
	}
	if _, exists := r.joinCodeIndex[room.JoinCode]; exists {
		return domain.ErrRoomAlreadyExists
	}

	// Enforce capacity
	r.enforceCapacity()

	// Store
	r.rooms[room.ID] = room
	r.joinCodeIndex[room.JoinCode] = room
	r.touch(room.ID)

	return nil
}

// GetByID returns a room and updates access time.
func (r *roomRepository) GetByID(ctx context.Context, id string) (*domain.Room, error) {
	if id == "" {
		return nil, domain.ErrInvalidInput
	}

	r.mu.RLock()
	room, exists := r.rooms[id]
	r.mu.RUnlock()
	if !exists {
		return nil, domain.ErrRoomNotFound
	}

	r.mu.Lock()
	r.touch(id)
	r.mu.Unlock()

	return room, nil
}

// GetByJoinCode returns a room by join code and updates access time.
func (r *roomRepository) GetByJoinCode(ctx context.Context, joinCode string) (*domain.Room, error) {
	if joinCode == "" {
		return nil, domain.ErrInvalidInput
	}

	r.mu.RLock()
	room, exists := r.joinCodeIndex[joinCode]
	r.mu.RUnlock()
	if !exists {
		return nil, domain.ErrRoomNotFound
	}

	r.mu.Lock()
	r.touch(room.ID)
	r.mu.Unlock()

	return room, nil
}

// Delete removes a room by pointer (idempotent).
func (r *roomRepository) Delete(ctx context.Context, room *domain.Room) (*domain.Room, error) {
	if room == nil || room.ID == "" {
		return nil, domain.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	storedRoom, exists := r.rooms[room.ID]
	if !exists {
		return nil, domain.ErrRoomNotFound
	}

	delete(r.rooms, room.ID)
	delete(r.joinCodeIndex, room.JoinCode)
	delete(r.lastAccess, room.ID)

	return storedRoom, nil
}

func (r *roomRepository) Update(ctx context.Context, room *domain.Room) error {
	if room == nil || room.ID == "" || room.JoinCode == "" {
		return domain.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existingRoom, exists := r.rooms[room.ID]
	if !exists {
		return domain.ErrRoomNotFound
	}

	if existingRoom.JoinCode != room.JoinCode {
		return domain.ErrInvalidInput
	}

	if existingRoom.ID != room.ID {
		return domain.ErrInvalidInput
	}

	r.evictIdle()

	r.rooms[room.ID] = room

	r.touch(room.ID)

	return nil
}

func (r *roomRepository) RemoveMember(ctx context.Context, roomID string, memberID string) (*domain.Member, error) {
	room, err := r.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	existingMember := room.FindMemberByID(memberID)
	if existingMember == nil {
		return nil, domain.ErrMemberNotFound
	}

	if err := room.LeaveAndAutoPromote(existingMember); err != nil {
		return nil, err
	}

	return existingMember, nil
}
