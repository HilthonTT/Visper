package rooms

import "time"

type createRoomRequest struct {
	Persistent bool   `json:"persistent"`
	Username   string `json:"username"`
}

type createRoomResponse struct {
	RoomID     string    `json:"roomId"`
	JoinCode   string    `json:"joinCode"`
	CreatedAt  time.Time `json:"createdAt"`
	Persistent bool      `json:"persistent"`
}

type bootUserRequest struct {
	MemberID string `json:"memberId"`
}

type userResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type messageResponse struct {
	ID      string       `json:"id"`
	User    userResponse `json:"user"`
	Content string       `json:"content"`
}

type roomResponse struct {
	ID         string            `json:"id"`
	JoinCode   string            `json:"joinCode"`
	Owner      userResponse      `json:"owner"`
	Persistent bool              `json:"persistent"`
	CreatedAt  time.Time         `json:"createdAt"`
	Messages   []messageResponse `json:"messages"`
}
