package room

import "time"

type CreateRoomRequest struct {
	ExpiryHrs int `json:"expiry_hours" binding:"required,min=1,max=168"` // 1 hour to 7 days
}

type JoinRoomRequest struct {
	Username string `json:"username" binding:"required"`
}

type JoinByCodeRequest struct {
	JoinCode string `json:"join_code" binding:"required,len=6"`
	Username string `json:"username" binding:"required"`
}

type RoomResponse struct {
	ID        string         `json:"id"`
	JoinCode  string         `json:"join_code"`
	Owner     UserResponse   `json:"owner"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt time.Time      `json:"expires_at"`
	Members   []UserResponse `json:"members"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type SuccessResponse struct {
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}
