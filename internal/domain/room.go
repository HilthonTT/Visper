package domain

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxMembers     = 10
	joinCodeLength = 6

	joinCodeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var (
	charsetLen = big.NewInt(int64(len(joinCodeChars)))

	ErrRoomNotFound      = errors.New("room not found")
	ErrRoomAlreadyExists = errors.New("room already exists")
	ErrRoomFull          = errors.New("room is full")
	ErrMemberNotFound    = errors.New("member not found")
	ErrRoomClosed        = errors.New("room is closed")
	ErrAlreadyInRoom     = errors.New("already in room")
)

type Room struct {
	ID         string        `json:"id"`
	JoinCode   string        `json:"joinCode"`
	Owner      *Member       `json:"owner"`
	Persistent bool          `json:"persistent"`
	CreatedAt  time.Time     `json:"createdAt"`
	Expiry     time.Duration `json:"expiry"`
	Members    []Member      `json:"members"`
}

type RoomRepository interface {
	Create(ctx context.Context, room *Room) error
	GetByID(ctx context.Context, id string) (*Room, error)
	GetByJoinCode(ctx context.Context, joinCode string) (*Room, error)
	Delete(ctx context.Context, room *Room) (*Room, error)
	Update(ctx context.Context, room *Room) error
	RemoveMember(ctx context.Context, roomID string, memberID string) (*Member, error)
}

func NewRoom(owner *Member, persistent bool, expiry time.Duration) (*Room, error) {
	now := time.Now()

	code, err := generateJoinCode()
	if err != nil {
		return nil, err
	}

	room := &Room{
		ID:         uuid.NewString(),
		JoinCode:   code,
		Members:    make([]Member, 0, maxMembers),
		Persistent: persistent,
		Owner:      owner,
		CreatedAt:  now,
		Expiry:     expiry,
	}

	if err := room.AddMember(owner); err != nil {
		return nil, err
	}

	return room, nil
}

func (r *Room) IsOwner(member *Member) bool {
	if r.Owner == nil || member == nil {
		return false
	}

	return r.Owner.User.ID == member.User.ID
}

func (r *Room) AddMember(m *Member) error {
	if m == nil {
		return ErrMemberNotFound
	}
	if len(r.Members) >= maxMembers {
		return ErrRoomFull
	}
	for _, existing := range r.Members {
		if existing.Token == m.Token {
			return ErrAlreadyInRoom
		}
	}
	r.Members = append(r.Members, *m)
	return nil
}

func (r *Room) IsMember(m *Member) bool {
	if m == nil {
		return false
	}

	for _, member := range r.Members {
		if member.User.ID == m.User.ID {
			return true
		}
	}

	return false
}

func (r *Room) FindMemberByID(memberID string) *Member {
	for _, m := range r.Members {
		if m.Token == memberID {
			return &m
		}
	}
	return nil
}

func (r *Room) RemoveMember(m *Member) error {
	return r.LeaveAndAutoPromote(m)
}

func (r *Room) LeaveAndAutoPromote(m *Member) error {
	if m == nil {
		return ErrMemberNotFound
	}

	idx := -1
	for i, mem := range r.Members {
		if mem.User.ID == m.User.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrMemberNotFound
	}

	// Remove the member (swap-remove to keep O(1))
	r.Members[idx] = r.Members[len(r.Members)-1]
	r.Members = r.Members[:len(r.Members)-1]

	// If the owner left, promote the first remaining member (if any)
	if r.Owner != nil && r.Owner.User.ID == m.User.ID {
		if len(r.Members) > 0 {
			r.Owner = &r.Members[0]
		} else {
			r.Owner = nil // room becomes owner-less (could be closed elsewhere)
		}
	}
	return nil
}

func generateJoinCode() (string, error) {
	var sb strings.Builder
	sb.Grow(joinCodeLength)

	for i := 0; i < joinCodeLength; i++ {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		sb.WriteByte(joinCodeChars[n.Int64()])
	}

	return sb.String(), nil
}
