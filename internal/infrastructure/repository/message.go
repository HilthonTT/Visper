package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/internal/domain"
)

// Oldest messages are evicted when capacity is exceeded.
type messageRepository struct {
	messages map[string][]domain.Message // roomID -> []Message
	capacity uint
	mu       *sync.RWMutex
}

func NewMessageRepository(capacity uint) domain.MessageRepository {
	if capacity == 0 {
		capacity = 100 // sane default
	}
	return &messageRepository{
		capacity: capacity,
		messages: make(map[string][]domain.Message),
		mu:       &sync.RWMutex{},
	}
}

func (r *messageRepository) Create(ctx context.Context, message *domain.Message) error {
	if message == nil || message.RoomID == "" {
		return domain.ErrInvalidInput
	}

	// Generate ID if not set
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	roomMsgs, exists := r.messages[message.RoomID]
	if !exists {
		roomMsgs = make([]domain.Message, 0, r.capacity)
	}

	roomMsgs = append(roomMsgs, *message)

	// Evict oldest if over capacity
	if len(roomMsgs) > int(r.capacity) {
		excess := len(roomMsgs) - int(r.capacity)
		roomMsgs = roomMsgs[excess:] // drop oldest
	}

	r.messages[message.RoomID] = roomMsgs

	return nil
}

func (r *messageRepository) Delete(ctx context.Context, message *domain.Message) error {
	if message.ID == "" || message.RoomID == "" {
		return domain.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	roomMsgs, exists := r.messages[message.RoomID]
	if !exists {
		return nil // idempotent: already gone
	}

	// Find and remove
	for i, msg := range roomMsgs {
		if msg.ID == message.ID {
			// Swap-remove
			roomMsgs[i] = roomMsgs[len(roomMsgs)-1]
			roomMsgs = roomMsgs[:len(roomMsgs)-1]
			r.messages[message.RoomID] = roomMsgs
			return nil
		}
	}

	return nil
}

func (r *messageRepository) GetByRoomID(ctx context.Context, roomID string) ([]domain.Message, error) {
	if roomID == "" {
		return nil, domain.ErrInvalidInput
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	roomMsgs, exists := r.messages[roomID]
	if !exists || len(roomMsgs) == 0 {
		return []domain.Message{}, nil
	}

	// Return a copy to prevent external mutation
	cpy := make([]domain.Message, len(roomMsgs))
	copy(cpy, roomMsgs)

	return cpy, nil
}
