package events

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hilthontt/visper/internal/domain"
	"github.com/hilthontt/visper/internal/infrastructure/contracts"
	"github.com/hilthontt/visper/internal/infrastructure/messaging"
	"github.com/rabbitmq/amqp091-go"
)

type roomConsumer struct {
	rabbitmq  *messaging.RabbitMQ
	auditRepo domain.RoomAuditRepository
}

func NewRoomConsumer(rabbitmq *messaging.RabbitMQ, auditRepo domain.RoomAuditRepository) *roomConsumer {
	return &roomConsumer{
		rabbitmq:  rabbitmq,
		auditRepo: auditRepo,
	}
}

func (c *roomConsumer) Listen() error {
	return c.rabbitmq.ConsumeMessages(messaging.RoomsQueue, func(ctx context.Context, msg amqp091.Delivery) error {
		var message contracts.AmqpMessage
		if err := json.Unmarshal(msg.Body, &message); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			return err
		}

		var payload messaging.RoomEventData
		if err := json.Unmarshal(message.Data, &payload); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			return err
		}

		var log *domain.RoomAuditLog

		switch msg.RoutingKey {
		case contracts.EventRoomCreated:
			log = domain.NewRoomCreatedLog(payload.Room.ID, payload.Room.Persistent, payload.Room.Expiry)
		case contracts.EventRoomDeleted:
			log = domain.NewRoomDeletedLog(payload.Room.ID, "deleted", len(payload.Room.Members))
		case contracts.EventMemberJoined:
			log = domain.NewMemberJoinedLog(payload.Room.ID, len(payload.Room.Members))
		case contracts.EventMemberKicked:
		case contracts.EventMemberLeft:
			log = domain.NewMemberLeftLog(payload.Room.ID, len(payload.Room.Members), false)
		}

		if log != nil {
			if err := c.auditRepo.Log(ctx, log); err != nil {
				return err
			}
		}

		return nil
	})
}
