package messages

// createMessageRequest represents the request to create a new message
type createMessageRequest struct {
	RoomID  string `json:"roomId" example:"550e8400-e29b-41d4-a716-446655440003"`             // Room ID
	Content string `json:"content" example:"Hello, everyone!" minLength:"1" maxLength:"5000"` // Message content
}

// createMessageResponse represents the response after creating a message
// createMessageResponse represents the response after creating a message
type createMessageResponse struct {
	ID      string `json:"id" example:"550e8400-e29b-41d4-a716-446655440003"`     // Unique message identifier
	RoomID  string `json:"roomId" example:"550e8400-e29b-41d4-a716-446655440000"` // Room identifier where message was sent
	Content string `json:"content" example:"Hello, everyone!"`                    // Message content
}
