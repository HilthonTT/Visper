package websocket

const (
	MemberJoined = "member.joined"
	MemberLeft   = "member.left"
	MemberList   = "member.list"

	MessageReceived = "message.received"
	MessageDeleted  = "message.deleted"
	MessageUpdated  = "message.updated"
	// MessageReactions carries a message's full, new list of reactions.
	MessageReactions = "message.reactions"

	// PresenceUpdate lists the members connected to the room right now.
	PresenceUpdate = "presence.update"
	// MemberTyping is relayed when a member types; clients expire it.
	MemberTyping = "member.typing"
	// ReadUpdate is the last message a member has read.
	ReadUpdate = "read.update"

	ErrorEvent          = "error"
	AuthenticationError = "error.auth"
	JoinFailed          = "error.join"
	RateLimited         = "error.rate_limited"
	Kicked              = "error.kicked"

	RoomDeleted = "room.deleted"
	RoomUpdated = "room.updated"
)
