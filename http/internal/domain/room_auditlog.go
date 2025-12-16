package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type RoomEventType string

const (
	EventRoomCreated       RoomEventType = "room_created"
	EventRoomDeleted       RoomEventType = "room_deleted"
	EventRoomExpired       RoomEventType = "room_expired"
	EventMemberJoined      RoomEventType = "member_joined"
	EventMemberLeft        RoomEventType = "member_left"
	EventMemberKicked      RoomEventType = "member_kicked"
	EventOwnershipTransfer RoomEventType = "ownership_transferred"
	EventRoomFull          RoomEventType = "room_full_rejected"
)

type RoomAuditLog struct {
	ID        string         `bson:"_id" json:"id"`
	RoomID    string         `bson:"room_id" json:"roomId"`
	EventType RoomEventType  `bson:"event_type" json:"eventType"`
	Timestamp time.Time      `bson:"timestamp" json:"timestamp"`
	Metadata  map[string]any `bson:"metadata,omitempty" json:"metadata,omitempty"`
}

type RoomAuditRepository interface {
	Log(ctx context.Context, log *RoomAuditLog) error
	GetByRoomID(ctx context.Context, roomID string, limit int) ([]RoomAuditLog, error)
	GetByEventType(ctx context.Context, eventType RoomEventType, from, to time.Time) ([]RoomAuditLog, error)
	DeleteOlderThan(ctx context.Context, before time.Time) error
	EnsureIndexes(ctx context.Context) error
}

func NewRoomCreatedLog(roomID string, persistent bool, expiry time.Duration) *RoomAuditLog {
	return &RoomAuditLog{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		EventType: EventRoomCreated,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"persistent":     persistent,
			"expiry_seconds": expiry.Seconds(),
		},
	}
}

func NewRoomDeletedLog(roomID string, reason string, memberCount int) *RoomAuditLog {
	return &RoomAuditLog{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		EventType: EventRoomDeleted,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"reason":       reason,
			"member_count": memberCount,
		},
	}
}

func NewMemberJoinedLog(roomID string, memberCount int) *RoomAuditLog {
	return &RoomAuditLog{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		EventType: EventMemberJoined,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"member_count": memberCount,
		},
	}
}

func NewMemberLeftLog(roomID string, memberCount int, wasOwner bool) *RoomAuditLog {
	return &RoomAuditLog{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		EventType: EventMemberLeft,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"member_count": memberCount,
			"was_owner":    wasOwner,
		},
	}
}

func NewOwnershipTransferLog(roomID string, reason string) *RoomAuditLog {
	return &RoomAuditLog{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		EventType: EventOwnershipTransfer,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"reason": reason, // "owner_left", "manual_transfer", etc.
		},
	}
}

func NewRoomFullRejectionLog(roomID string) *RoomAuditLog {
	return &RoomAuditLog{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		EventType: EventRoomFull,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{},
	}
}
