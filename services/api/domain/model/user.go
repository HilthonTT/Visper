package model

import "time"

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	IsGuest   bool      `json:"isGuest"`
	CreatedAt time.Time `json:"created_at"`
	// SeenAt is when the user's record was last refreshed; records expire
	// after a period of inactivity.
	SeenAt time.Time `json:"seen_at,omitzero"`
}
