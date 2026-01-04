package websocket

type WSMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomId"`
	Data   any    `json:"data"`
}

type MessagePayload struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Timestamp string `json:"timestamp"`
}

type MemberPayload struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	JoinedAt string `json:"joinedAt,omitempty"`
}

type RoomDeletedPayload struct {
	RoomID string `json:"roomid"`
}

func NewMessageReceived(roomID, msgID, content, userID, username, timestamp string) *WSMessage {
	return &WSMessage{
		Type:   MessageReceived,
		RoomID: roomID,
		Data: MessagePayload{
			ID:        msgID,
			Content:   content,
			UserID:    userID,
			Username:  username,
			Timestamp: timestamp,
		},
	}
}

func NewMemberJoined(roomID string, member MemberPayload) *WSMessage {
	return &WSMessage{
		Type:   MemberJoined,
		RoomID: roomID,
		Data:   member,
	}
}

func NewMemberLeft(roomID, userID, username string) *WSMessage {
	return &WSMessage{
		Type:   MemberLeft,
		RoomID: roomID,
		Data: MemberPayload{
			UserID:   userID,
			Username: username,
		},
	}
}

func NewRoomDeleted(roomID string) *WSMessage {
	return &WSMessage{
		Type:   RoomDeleted,
		RoomID: roomID,
		Data: RoomDeletedPayload{
			RoomID: roomID,
		},
	}
}
