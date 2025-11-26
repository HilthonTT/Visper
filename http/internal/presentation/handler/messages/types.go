package messages

type createMessageRequest struct {
	RoomID  string `json:"roomId"`
	Content string `json:"content"`
}

type createMessageResponse struct {
	ID      string `json:"id"`
	RoomID  string `json:"roomId"`
	Content string `json:"content"`
}
