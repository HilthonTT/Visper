package contracts

// AmqpMessage is the message structure for AMQP.
type AmqpMessage struct {
	OwnerID string `json:"ownerId"`
	Data    []byte `json:"data"`
}

type EventType string

// Routing keys - using consistent event/command patterns
const (
	EventMessageSent  EventType = "message.sent"
	EventMemberJoined EventType = "member.joined"
	EventMemberLeft   EventType = "member.left"
	EventRoomCreated  EventType = "room.created"
	EventRoomDeleted  EventType = "room.deleted"
)
