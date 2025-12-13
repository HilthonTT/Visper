package events

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hilthontt/visper/internal/infrastructure/contracts"
	"github.com/hilthontt/visper/internal/infrastructure/messaging"
	"github.com/rabbitmq/amqp091-go"
)

type roomConsumer struct {
	rabbitmq *messaging.RabbitMQ
}

func NewRoomConsumer(rabbitmq *messaging.RabbitMQ) *roomConsumer {
	return &roomConsumer{
		rabbitmq: rabbitmq,
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

		// TODO: Write audit logs

		log.Printf("Room Event Data received: %+v", payload)

		return nil
	})
}
