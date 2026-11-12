package model

import (
	"net/url"
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
	if userID == "" {
		return false
	}

	if r.Owner.ID == userID {
		return true
	}

	for _, user := range r.Members {
		if user.ID == userID {
			return true
		}
	}

	return false
}

func (r Room) GetQRCodeURL(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	q := u.Query()
	q.Set("joinCode", r.JoinCode)
	q.Set("secureCode", r.SecureCode)
	u.RawQuery = q.Encode()

	return u.String()
}

// ExpiresAt returns when the room expires, or the zero time for a room that
// never does.
func (r Room) ExpiresAt() time.Time {
	if r.Expiry <= 0 {
		return time.Time{}
	}
	return r.CreatedAt.Add(r.Expiry)
}

func (r Room) HasExpired() bool {
	expiresAt := r.ExpiresAt()
	return !expiresAt.IsZero() && time.Now().After(expiresAt)
}
