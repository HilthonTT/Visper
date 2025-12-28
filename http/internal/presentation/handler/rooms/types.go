package rooms

import "time"

// createRoomRequest represents the request to create a new chat room
type createRoomRequest struct {
	Persistent bool   `json:"persistent" example:"true"`                 // Whether the room persists after all users leave
	Username   string `json:"username" example:"john_doe" minLength:"1"` // Username of the room creator
}

// createRoomResponse represents the response after creating a room
type createRoomResponse struct {
	RoomID      string         `json:"roomId" example:"550e8400-e29b-41d4-a716-446655440000"` // Unique room identifier
	JoinCode    string         `json:"joinCode" example:"ABC123"`                             // Code required to join the room
	CreatedAt   time.Time      `json:"createdAt" example:"2024-01-01T12:00:00Z"`              // Room creation timestamp
	Persistent  bool           `json:"persistent" example:"true"`                             // Whether the room is persistent
	Members     []userResponse `json:"members"`                                               // List of members in the room
	MemberToken string         `json:"memberToken"`                                           // The Member Token
}

// bootUserRequest represents the request to remove a user from a room
type bootUserRequest struct {
	MemberID string `json:"memberId" example:"550e8400-e29b-41d4-a716-446655440001"` // Member ID to remove
}

// userResponse represents basic user information
type userResponse struct {
	ID   string `json:"id" example:"550e8400-e29b-41d4-a716-446655440002"` // Unique user identifier
	Name string `json:"name" example:"john_doe"`                           // Username
}

// messageResponse represents a chat message
type messageResponse struct {
	ID        string       `json:"id" example:"550e8400-e29b-41d4-a716-446655440003"` // Unique message identifier
	User      userResponse `json:"user"`                                              // User who sent the message
	Content   string       `json:"content" example:"Hello, world!"`                   // Message content
	CreatedAt time.Time    `json:"createdAt"`                                         // Message timestamp
}

// roomResponse represents detailed room information
type roomResponse struct {
	ID         string            `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"` // Unique room identifier
	JoinCode   string            `json:"joinCode" example:"ABC123"`                         // Code required to join the room
	Owner      userResponse      `json:"owner"`                                             // Room owner information
	Persistent bool              `json:"persistent" example:"true"`                         // Whether the room is persistent
	CreatedAt  time.Time         `json:"createdAt" example:"2024-01-01T12:00:00Z"`          // Room creation timestamp
	Messages   []messageResponse `json:"messages"`                                          // List of messages in the room
	Members    []userResponse    `json:"members"`                                           // List of members in the room
}
