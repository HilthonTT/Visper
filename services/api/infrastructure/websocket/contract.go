package websocket

import (
	"github.com/hilthontt/visper/services/api/domain/model"
)

type WSMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomId"`
	Data   any    `json:"data"`
}

type MessagePayload struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	UserID    string            `json:"userId"`
	Username  string            `json:"username"`
	Timestamp string            `json:"timestamp"`
	Encrypted bool              `json:"encrypted"`
	ReplyToID string            `json:"replyToId,omitempty"`
	Reactions []ReactionPayload `json:"reactions,omitempty"`
}

type ReactionPayload struct {
	UserID    string `json:"userId"`
	Emoji     string `json:"emoji"`
	Encrypted bool   `json:"encrypted"`
}

type MessageReactionsPayload struct {
	ID        string            `json:"id"`
	Reactions []ReactionPayload `json:"reactions"`
}

type PresencePayload struct {
	// Online holds the IDs of the members connected right now.
	Online []string `json:"online"`
}

type TypingPayload struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

type ReadPayload struct {
	UserID    string `json:"userId"`
	MessageID string `json:"messageId"`
}

type MessageUpdatedPayload struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	RoomID    string `json:"roomId"`
	Timestamp string `json:"timestamp"`
	Encrypted bool   `json:"encrypted"`
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

type ErrorKickedPayload struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
}

func NewMessageReceived(roomID string, m *model.Message) *WSMessage {
	return &WSMessage{
		Type:   MessageReceived,
		RoomID: roomID,
		Data: MessagePayload{
			ID:       m.ID,
			Content:  m.Content,
			UserID:   m.UserID,
			Username: m.Username,
			// Millisecond precision so clients can order messages sent within the same second.
			Timestamp: m.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			Encrypted: m.Encrypted,
			ReplyToID: m.ReplyToID,
			Reactions: toReactionPayloads(m.Reactions),
		},
	}
}

func NewMessageReactions(roomID string, m *model.Message) *WSMessage {
	reactions := toReactionPayloads(m.Reactions)
	if reactions == nil {
		reactions = []ReactionPayload{}
	}
	return &WSMessage{
		Type:   MessageReactions,
		RoomID: roomID,
		Data:   MessageReactionsPayload{ID: m.ID, Reactions: reactions},
	}
}

func toReactionPayloads(reactions []model.Reaction) []ReactionPayload {
	if len(reactions) == 0 {
		return nil
	}
	out := make([]ReactionPayload, len(reactions))
	for i, r := range reactions {
		out[i] = ReactionPayload{UserID: r.UserID, Emoji: r.Emoji, Encrypted: r.Encrypted}
	}
	return out
}

func NewPresenceUpdate(roomID string, online []string) *WSMessage {
	if online == nil {
		online = []string{}
	}
	return &WSMessage{Type: PresenceUpdate, RoomID: roomID, Data: PresencePayload{Online: online}}
}

func NewMemberTyping(roomID, userID, username string) *WSMessage {
	return &WSMessage{Type: MemberTyping, RoomID: roomID, Data: TypingPayload{UserID: userID, Username: username}}
}

func NewReadUpdate(roomID, userID, messageID string) *WSMessage {
	return &WSMessage{Type: ReadUpdate, RoomID: roomID, Data: ReadPayload{UserID: userID, MessageID: messageID}}
}

func NewMessageUpdated(roomID, msgID, content, timestamp string, encrypted bool) *WSMessage {
	return &WSMessage{
		Type:   MessageUpdated,
		RoomID: roomID,
		Data: MessageUpdatedPayload{
			ID:        msgID,
			RoomID:    roomID,
			Content:   content,
			Timestamp: timestamp,
			Encrypted: encrypted,
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

func NewErrorKicked(roomID, kickedUserID, kickedUsername, reason string) *WSMessage {
	return &WSMessage{
		Type:   Kicked,
		RoomID: roomID,
		Data: ErrorKickedPayload{
			UserID:   kickedUserID,
			Username: kickedUsername,
			Reason:   reason,
		},
	}
}
