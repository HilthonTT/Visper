package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/broker"
)

// EventConsumer consumes and processes events
type EventConsumer struct {
	consumer           *broker.Consumer
	handlers           map[EventType]EventHandler
	stopCh             chan struct{}
	auditLogRepository repository.AuditLogRepository
}

// EventHandler is a function that handles a specific event type
type EventHandler func(event *Event) error

// NewEventConsumer creates a new event consumer
func NewEventConsumer(brokerInstance *broker.Broker, groupID, topic string, auditLogRepository repository.AuditLogRepository) (*EventConsumer, error) {
	consumer := broker.NewConsumer(brokerInstance, groupID)

	// Subscribe to topic
	if err := consumer.Subscribe(topic); err != nil {
		return nil, fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	ec := &EventConsumer{
		consumer:           consumer,
		handlers:           make(map[EventType]EventHandler),
		stopCh:             make(chan struct{}),
		auditLogRepository: auditLogRepository,
	}

	// Register default handlers
	ec.RegisterHandler(EventRoomCreated, ec.handleRoomCreated)
	ec.RegisterHandler(EventRoomJoined, ec.handleRoomJoined)
	ec.RegisterHandler(EventMessageSent, ec.handleMessageSent)
	ec.RegisterHandler(EventRoomExpired, ec.handleRoomExpired)
	ec.RegisterHandler(EventUserLeft, ec.handleUserLeft)

	return ec, nil
}

// RegisterHandler registers a handler for a specific event type
func (ec *EventConsumer) RegisterHandler(eventType EventType, handler EventHandler) {
	ec.handlers[eventType] = handler
}

// Start starts consuming events
func (ec *EventConsumer) Start() {
	log.Println("Event consumer started")

	for {
		select {
		case <-ec.stopCh:
			log.Println("Event consumer stopped")
			return
		default:
			// Poll for new messages
			records, err := ec.consumer.Poll(100) // 100ms timeout
			if err != nil {
				log.Printf("Error polling messages: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			// Process each record
			for _, record := range records {
				if err := ec.processRecord(record); err != nil {
					log.Printf("Error processing record: %v", err)
				}
			}

			// Small delay if no messages
			if len(records) == 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

// Stop stops the consumer
func (ec *EventConsumer) Stop() {
	close(ec.stopCh)
}

// processRecord processes a single consumer record
func (ec *EventConsumer) processRecord(record *broker.ConsumerRecord) error {
	var event Event
	if err := json.Unmarshal(record.Value, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	handler, exists := ec.handlers[event.Type]
	if !exists {
		log.Printf("No handler registered for event type: %s", event.Type)
		return nil
	}

	handlerErr := handler(&event)
	if err := ec.writeAuditLog(&event, handlerErr); err != nil {
		log.Printf("Failed to write audit log for event %s: %v", event.ID, err)
	}

	return handlerErr
}

func (ec *EventConsumer) handleRoomCreated(event *Event) error {
	expiresIn := event.Data["expires_in_seconds"]
	log.Printf("Room created: %s by user %s (expires in %.0f seconds)",
		event.RoomID, event.UserID, expiresIn)

	return nil
}

func (ec *EventConsumer) handleRoomJoined(event *Event) error {
	log.Printf("User %s joined room %s", event.UserID, event.RoomID)

	return nil
}

func (ec *EventConsumer) handleMessageSent(event *Event) error {
	messageID := event.Data["message_id"]
	messageSize := event.Data["message_size"]

	log.Printf("Message sent in room %s by user %s (id: %s, size: %v bytes)",
		event.RoomID, event.UserID, messageID, messageSize)

	return nil
}

func (ec *EventConsumer) handleRoomExpired(event *Event) error {
	messageCount := event.Data["message_count"]
	log.Printf("Room expired: %s (total messages: %v)", event.RoomID, messageCount)

	return nil
}

func (ec *EventConsumer) handleUserLeft(event *Event) error {
	log.Printf("User %s left room %s", event.UserID, event.RoomID)

	return nil
}

func (ec *EventConsumer) writeAuditLog(event *Event, handlerErr error) error {
	payload, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	entry := model.AuditLog{
		EventID:   event.ID,
		EventType: string(event.Type),
		CreatedAt: event.Timestamp,
		UserID:    event.UserID,
		Payload:   payload,
		Success:   handlerErr == nil,
	}

	if event.RoomID != "" {
		entry.RoomID = sql.NullString{Valid: true, String: event.RoomID}
	}

	if handlerErr != nil {
		entry.ErrorMessage = sql.NullString{Valid: true, String: handlerErr.Error()}
	}

	_, err = ec.auditLogRepository.CreateAuditLog(context.Background(), entry)
	if err != nil {
		return err
	}
	return nil
}
