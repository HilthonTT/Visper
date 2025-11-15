package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/internal/infrastructure/validate"
)

type User struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Joined time.Time `json:"joined"`
}

func NewUser(rawName string) (*User, error) {
	validateUsername := validate.Compose(
		validate.Required(),
		validate.MinLength(2),
		validate.MaxLength(32),
		validate.NoSpaces(),
		// Allow letters, numbers, underscore, hyphen
		validate.Matches(`^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$`,
			"username can only contain letters, numbers, underscores, and hyphens (cannot start/end with _ or -)"),
	)

	if err := validateUsername(rawName); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(rawName)
	name = strings.ToLower(name)
	now := time.Now()

	return &User{
		ID:     uuid.NewString(),
		Name:   name,
		Joined: now,
	}, nil
}
