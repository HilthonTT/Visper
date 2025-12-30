package model

import "time"

type Room struct {
	ID        string        `json:"id"`
	JoinCode  string        `json:"joinCode"`
	Owner     User          `json:"owner"`
	CreatedAt time.Time     `json:"createdAt"`
	Expiry    time.Duration `json:"expiry"`
	Members   []User        `json:"members"`
}
