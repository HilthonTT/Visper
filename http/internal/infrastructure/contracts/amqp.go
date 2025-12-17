package contracts

// AmqpMessage is the message structure for AMQP.
type AmqpMessage struct {
	OwnerID string `json:"ownerId"`
	Data    []byte `json:"data"`
}

// Routing keys - using consistent event/command patterns
const (
	EventMessageSent  = "message.sent"
	EventMemberJoined = "member.joined"
	EventMemberLeft   = "member.left"
	EventMemberKicked = "member.kicked"
	EventRoomCreated  = "room.created"
	EventRoomDeleted  = "room.deleted"
)
