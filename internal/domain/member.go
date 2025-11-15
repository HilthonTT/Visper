package domain

type Member struct {
	Token  string `json:"token"`
	User   *User  `json:"user"`
	RoomID string `json:"roomId"`
}
