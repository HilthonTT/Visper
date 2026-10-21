package model

import (
	"database/sql"
	"time"

	"gorm.io/gorm"
)

type BaseModel struct {
	ID int `gorm:"primaryKey"`

	CreatedAt  time.Time    `gorm:"type:TIMESTAMP with time zone;not null"`
	ModifiedAt sql.NullTime `gorm:"type:TIMESTAMP with time zone;null"`
	DeletedAt  sql.NullTime `gorm:"type:TIMESTAMP with time zone;null"`

	CreatedBy  int             `gorm:"not null"`
	ModifiedBy *sql.NullString `gorm:"null"`
	DeletedBy  *sql.NullString `gorm:"null"`
}

// actorKey is the context key holding the acting user's ID (a string).
const actorKey = "user"

// actor returns the acting user's ID from the statement context, if any.
// The type assertion is checked so an unexpected value cannot panic.
func actor(tx *gorm.DB) *sql.NullString {
	if id, ok := tx.Statement.Context.Value(actorKey).(string); ok && id != "" {
		return &sql.NullString{Valid: true, String: id}
	}
	return &sql.NullString{Valid: false}
}

// BeforeCreate is a GORM hook; it must return an error to be recognised.
func (m *BaseModel) BeforeCreate(tx *gorm.DB) error {
	m.CreatedAt = time.Now().UTC()
	m.CreatedBy = -1
	return nil
}

func (m *BaseModel) BeforeUpdate(tx *gorm.DB) error {
	m.ModifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	m.ModifiedBy = actor(tx)
	return nil
}

func (m *BaseModel) BeforeDelete(tx *gorm.DB) error {
	m.DeletedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	m.DeletedBy = actor(tx)
	return nil
}
