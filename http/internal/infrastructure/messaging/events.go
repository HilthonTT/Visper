package messaging

import "github.com/hilthontt/visper/internal/domain"

const (
	MessagesQueue   = "messages"
	RoomsQueue      = "rooms"
	DeadLetterQueue = "dead_letter_queue"
)

type RoomEventData struct {
	Room domain.Room `json:"room"`
}