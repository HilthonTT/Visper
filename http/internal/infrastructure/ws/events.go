package ws

const (
	MemberJoined = "member.joined"
	MemberLeft   = "member.left"
	MemberList   = "member.list"

	MessageReceived = "member.received"
	MessageDeleted  = "member.deleted"

	ErrorEvent          = "error"
	AuthenticationError = "error.auth"
	JoinFailed          = "error.join"
	RateLimited         = "error.rate_limited"
	Kicked              = "error.kicked"

	RoomDeleted = "room.deleted"
)
