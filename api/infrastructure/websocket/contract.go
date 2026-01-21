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

type MessageUpdatedPayload struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	RoomID    string `json:"roomId"`
	Timestamp string `json:"timestamp"`
}

type MessageDeletedPayload struct {
	ID        string `json:"id"`
	RoomID    string `json:"roomId"`
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

type RoomUpdatedPayload struct {
	RoomID   string `json:"roomId"`
	JoinCode string `json:"joinCode"`
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

func NewMessageUpdated(roomID, msgID, content, timestamp string) *WSMessage {
	return &WSMessage{
		Type:   MessageUpdated,
		RoomID: roomID,
		Data: MessageUpdatedPayload{
			ID:        msgID,
			RoomID:    roomID,
			Content:   content,
			Timestamp: timestamp,
		},
	}
}

func NewMessageDeleted(roomID, msgID, timestamp string) *WSMessage {
	return &WSMessage{
		Type:   MessageDeleted,
		RoomID: roomID,
		Data: MessageDeletedPayload{
			ID:        msgID,
			RoomID:    roomID,
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

func NewRoomUpdated(roomID, joinCode string) *WSMessage {
	return &WSMessage{
		Type:   RoomUpdated,
		RoomID: roomID,
		Data: RoomUpdatedPayload{
			RoomID:   roomID,
			JoinCode: joinCode,
		},
	}
}
