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

func (r Room) IsMember(userID string) bool {
	for _, user := range r.Members {
		if user.ID == userID {
			return true
		}
	}

	return false
}
