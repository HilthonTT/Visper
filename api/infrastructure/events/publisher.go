package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hilthontt/visper/api/infrastructure/broker"
)

// EventPublisher publishes Visper events to the broker
type EventPublisher struct {
	producer *broker.Producer
	topic    string
}

// NewEventPublisher creates a new event publisher
func NewEventPublisher(brokerInstance *broker.Broker, topic string) (*EventPublisher, error) {
	// Create topic if it doesn't exist
	if err := brokerInstance.CreateTopic(topic, 3); err != nil {
		// Topic might already exist, that's okay
		if err.Error() != fmt.Sprintf("topic %s already exists", topic) {
			return nil, fmt.Errorf("failed to create topic: %w", err)
		}
	}

	// Create producer with leader acknowledgment
	producer := broker.NewProducer(brokerInstance, 1)

	return &EventPublisher{
		producer: producer,
		topic:    topic,
	}, nil
}

// Publish publishes an event to the broker
func (ep *EventPublisher) Publish(event *Event) error {
	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	msg := &broker.Message{
		Key:       []byte(event.RoomID),
		Value:     eventJSON,
		Timestamp: event.Timestamp,
	}

	_, _, err = ep.producer.Produce(ep.topic, msg)
	if err != nil {
		return fmt.Errorf("failed to produce event: %w", err)
	}

	return nil
}

// PublishRoomCreated publishes a room created event
func (ep *EventPublisher) PublishRoomCreated(roomID, userID string, expiresIn time.Duration) error {
	event := &Event{
		ID:     generateEventID(),
		Type:   EventRoomCreated,
		UserID: userID,
		RoomID: roomID,
		Data: map[string]interface{}{
			"expires_in_seconds": expiresIn.Seconds(),
		},
	}
	return ep.Publish(event)
}

// PublishRoomJoined publishes a room joined event
func (ep *EventPublisher) PublishRoomJoined(roomID, userID string) error {
	event := &Event{
		ID:     generateEventID(),
		Type:   EventRoomJoined,
		UserID: userID,
		RoomID: roomID,
	}
	return ep.Publish(event)
}

// PublishMessageSent publishes a message sent event
func (ep *EventPublisher) PublishMessageSent(roomID, userID, messageID string, messageSize int) error {
	event := &Event{
		ID:     generateEventID(),
		Type:   EventMessageSent,
		UserID: userID,
		RoomID: roomID,
		Data: map[string]any{
			"message_id":   messageID,
			"message_size": messageSize,
		},
	}
	return ep.Publish(event)
}

// PublishRoomExpired publishes a room expired event
func (ep *EventPublisher) PublishRoomExpired(roomID string, messageCount int) error {
	event := &Event{
		ID:     generateEventID(),
		Type:   EventRoomExpired,
		RoomID: roomID,
		Data: map[string]any{
			"message_count": messageCount,
		},
	}
	return ep.Publish(event)
}

// PublishUserLeft publishes a user left event
func (ep *EventPublisher) PublishUserLeft(roomID, userID string) error {
	event := &Event{
		ID:     generateEventID(),
		Type:   EventUserLeft,
		UserID: userID,
		RoomID: roomID,
	}
	return ep.Publish(event)
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}
