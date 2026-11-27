package websocket

import (
	"time"

	"github.com/hilthontt/visper/services/api/application/ports"
	"github.com/hilthontt/visper/services/api/domain/model"
)

// Notifier broadcasts use case changes to the room's websocket clients.
type Notifier struct {
	core *Core
}

var _ ports.Notifier = (*Notifier)(nil)

func NewNotifier(core *Core) *Notifier {
	return &Notifier{core: core}
}

func now() string { return time.Now().Format(time.RFC3339) }

func (n *Notifier) RoomUpdated(roomID, joinCode string) {
	n.core.Publish(NewRoomUpdated(roomID, joinCode))
}

func (n *Notifier) RoomDeleted(roomID string) {
	n.core.Publish(NewRoomDeleted(roomID))
}

func (n *Notifier) MemberJoined(roomID string, member model.User) {
	n.core.Publish(NewMemberJoined(roomID, MemberPayload{
		UserID:   member.ID,
		Username: member.Username,
		JoinedAt: now(),
	}))
}

func (n *Notifier) MemberLeft(roomID string, member model.User) {
	n.core.Publish(NewMemberLeft(roomID, member.ID, member.Username))
}

func (n *Notifier) MemberKicked(roomID string, member model.User, reason string) {
	n.core.Publish(NewErrorKicked(roomID, member.ID, member.Username, reason))
}

func (n *Notifier) MessageReceived(roomID string, message *model.Message) {
	n.core.Publish(NewMessageReceived(roomID, message))
}

func (n *Notifier) MessageUpdated(roomID string, message *model.Message) {
	n.core.Publish(NewMessageUpdated(roomID, message.ID, message.Content,
		message.UpdatedAt.Format(time.RFC3339), message.Encrypted))
}

func (n *Notifier) MessageDeleted(roomID, messageID string) {
	n.core.Publish(NewMessageDeleted(roomID, messageID, now()))
}

func (n *Notifier) MessageReactions(roomID string, message *model.Message) {
	n.core.Publish(NewMessageReactions(roomID, message))
}

func (n *Notifier) MemberCard(roomID, userID, card string) {
	n.core.Publish(NewMemberCard(roomID, userID, card))
}

func (n *Notifier) RoomRekeyed(roomID string, rekey *model.Rekey) {
	n.core.Publish(NewRoomRekeyed(roomID, rekey))
}
