package ws

type WSMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomId"`
	Data   any    `json:"data"`
}

// Payload structs
type MessagePayload struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Timestamp string `json:"timestamp"`
}

type MessageDeletedPayload struct {
	MessageID string `json:"id"`
}

type MemberPayload struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	JoinedAt string `json:"joinedAt,omitempty"`
}

type MemberListPayload struct {
	Members []MemberPayload `json:"members"`
}

type ErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Retry   bool   `json:"retry,omitempty"`
}

type RoomUpdatePayload struct {
	Name        string `json:"name,omitempty"`
	Topic       string `json:"topic,omitempty"`
	MaxMembers  int    `json:"maxMembers,omitempty"`
	MemberCount int    `json:"memberCount"`
}

type BootPayload struct {
	Username string `json:"username"`
	Reason   string `json:"reason"`
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

func NewMessageDeleted(roomID, msgID string) *WSMessage {
	return &WSMessage{
		Type:   MessageDeleted,
		RoomID: roomID,
		Data: MessageDeletedPayload{
			MessageID: msgID,
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

func NewMemberList(roomID string, members []MemberPayload) *WSMessage {
	return &WSMessage{
		Type:   MemberList,
		RoomID: roomID,
		Data:   MemberListPayload{Members: members},
	}
}

func NewError(roomID, message string) *WSMessage {
	return &WSMessage{
		Type:   ErrorEvent,
		RoomID: roomID,
		Data: ErrorPayload{
			Message: message,
			Retry:   false,
		},
	}
}

func NewAuthError(roomID, message string) *WSMessage {
	return &WSMessage{
		Type:   AuthenticationError,
		RoomID: roomID,
		Data: ErrorPayload{
			Code:    "AUTH_FAILED",
			Message: message,
			Retry:   true,
		},
	}
}

func NewJoinFailed(roomID, reason string) *WSMessage {
	return &WSMessage{
		Type:   JoinFailed,
		RoomID: roomID,
		Data: ErrorPayload{
			Code:    "JOIN_FAILED",
			Message: reason,
			Retry:   true,
		},
	}
}

func NewKicked(roomID, username, reason string) *WSMessage {
	return &WSMessage{
		Type:   Kicked,
		RoomID: roomID,
		Data: BootPayload{
			Reason:   reason,
			Username: username,
		},
	}
}
