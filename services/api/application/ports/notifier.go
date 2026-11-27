package ports

import "github.com/hilthontt/visper/services/api/domain/model"

// Notifier tells a room's connected clients what changed. The use cases call
// it after a change is stored, so every path that makes a change also
// announces it, whichever handler or job triggered it.
type Notifier interface {
	RoomUpdated(roomID, joinCode string)
	RoomDeleted(roomID string)

	MemberJoined(roomID string, member model.User)
	MemberLeft(roomID string, member model.User)
	// MemberKicked tells the room, and disconnects the kicked member.
	MemberKicked(roomID string, member model.User, reason string)

	MessageReceived(roomID string, message *model.Message)
	MessageUpdated(roomID string, message *model.Message)
	MessageDeleted(roomID, messageID string)
	MessageReactions(roomID string, message *model.Message)

	MemberCard(roomID, userID, card string)
	RoomRekeyed(roomID string, rekey *model.Rekey)
}

// NopNotifier is a Notifier that tells no one.
type NopNotifier struct{}

func (NopNotifier) RoomUpdated(string, string)              {}
func (NopNotifier) RoomDeleted(string)                      {}
func (NopNotifier) MemberJoined(string, model.User)         {}
func (NopNotifier) MemberLeft(string, model.User)           {}
func (NopNotifier) MemberKicked(string, model.User, string) {}
func (NopNotifier) MessageReceived(string, *model.Message)  {}
func (NopNotifier) MessageUpdated(string, *model.Message)   {}
func (NopNotifier) MessageDeleted(string, string)           {}
func (NopNotifier) MessageReactions(string, *model.Message) {}
func (NopNotifier) MemberCard(string, string, string)       {}
func (NopNotifier) RoomRekeyed(string, *model.Rekey)        {}
