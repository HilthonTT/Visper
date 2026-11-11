package websocket

type NotifySelfRoomInviteRequest struct {
	UserID string `json:"user_id" binding:"required"`
}
