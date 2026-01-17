package websocket

const (
	MemberJoined = "member.joined"
	MemberLeft   = "member.left"
	MemberList   = "member.list"

	MessageReceived = "message.received"
	MessageDeleted  = "message.deleted"
	MessageUpdated  = "message.updated"

	ErrorEvent          = "error"
	AuthenticationError = "error.auth"
	JoinFailed          = "error.join"
	RateLimited         = "error.rate_limited"
	Kicked              = "error.kicked"

	RoomDeleted = "room.deleted"
)
