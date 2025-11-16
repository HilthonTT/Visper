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
