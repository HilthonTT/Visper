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

func (m *BaseModel) BeforeCreate(tx *gorm.DB) {
	value := tx.Statement.Context.Value("user")
	userId := -1

	if value != nil {
		userId = int(value.(float64))
	}
	m.CreatedAt = time.Now().UTC()
	m.CreatedBy = userId
}

func (m *BaseModel) BeforeUpdate(tx *gorm.DB) (err error) {
	value := tx.Statement.Context.Value("user")
	var userId = &sql.NullString{Valid: false}
	if value != nil {
		userId = &sql.NullString{Valid: true, String: string(value.(string))}
	}
	m.ModifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	m.ModifiedBy = userId
	return
}

func (m *BaseModel) BeforeDelete(tx *gorm.DB) (err error) {
	value := tx.Statement.Context.Value("user")
	var userId = &sql.NullString{Valid: false}
	if value != nil {
		userId = &sql.NullString{Valid: true, String: string(value.(string))}
	}
	m.DeletedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	m.DeletedBy = userId
	return
}
