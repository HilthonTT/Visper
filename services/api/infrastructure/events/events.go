package events

import "time"

// EventType defines the type of event
type EventType string

const (
	EventRoomCreated EventType = "room.created"
	EventRoomJoined  EventType = "room.joined"
	EventMessageSent EventType = "message.sent"
	EventRoomExpired EventType = "room.expired"
	EventUserLeft    EventType = "user.left"
	EventRoomDeleted EventType = "room.deleted"
)

// Event represents a Visper application event
type Event struct {
	ID        string         `json:"id"`
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	UserID    string         `json:"user_id,omitempty"`
	RoomID    string         `json:"room_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}
