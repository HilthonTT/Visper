package ports

import "time"

// EventPublisher publishes analytics and audit events.
type EventPublisher interface {
	PublishRoomCreated(roomID, userID string, expiresIn time.Duration) error
	PublishRoomJoined(roomID, userID string) error
	PublishMessageSent(roomID, userID, messageID string, messageSize int) error
}
