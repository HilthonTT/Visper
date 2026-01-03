package model

import "time"

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	IsGuest   bool      `json:"isGuest"`
	CreatedAt time.Time `json:"created_at"`
}
