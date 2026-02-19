package model

import (
	"database/sql"
	"time"
)

type AuditLog struct {
	ID        int       `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"type:TIMESTAMP with time zone;not null;index"`

	// Event identity
	EventID   string `gorm:"type:VARCHAR(36);not null;uniqueIndex"`
	EventType string `gorm:"type:VARCHAR(64);not null;index"`

	// Actors - anonymous, so no FK constraints
	UserID string         `gorm:"type:VARCHAR(36);not null;index"`
	RoomID sql.NullString `gorm:"type:VARCHAR(36);null;index"`

	// Payload snapshot
	Payload []byte `gorm:"type:JSONB;not null"`

	// Outcome
	Success      bool           `gorm:"not null;default:true"`
	ErrorMessage sql.NullString `gorm:"type:TEXT;null"`
}
