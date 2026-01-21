package model

import (
	"fmt"
	"time"
)

type Room struct {
	ID         string        `json:"id"`
	JoinCode   string        `json:"joinCode"`
	SecureCode string        `json:"secureCode"`
	Owner      User          `json:"owner"`
	CreatedAt  time.Time     `json:"createdAt"`
	Expiry     time.Duration `json:"expiry"`
	Members    []User        `json:"members"`
}

func (r Room) IsMember(userID string) bool {
	for _, user := range r.Members {
		if user.ID == userID {
			return true
		}
	}

	return false
}

func (r Room) GetQRCodeURL(baseURL string) string {
	return fmt.Sprintf("%s/join/%s?token=%s", baseURL, r.JoinCode, r.SecureCode)
}
