package ws

type WSMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomId"`
	Data   any    `json:"data"`
}
