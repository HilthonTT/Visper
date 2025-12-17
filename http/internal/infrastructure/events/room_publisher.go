package events

import (
	"context"
	"encoding/json"

	"github.com/hilthontt/visper/internal/domain"
	"github.com/hilthontt/visper/internal/infrastructure/contracts"
	"github.com/hilthontt/visper/internal/infrastructure/messaging"
)

type RoomPublisher struct {
	rabbitmq *messaging.RabbitMQ
}

func NewRoomPublisher(rabbitmq *messaging.RabbitMQ) *RoomPublisher {
	return &RoomPublisher{
		rabbitmq: rabbitmq,
	}
}

func (p *RoomPublisher) PublishRoomCreated(ctx context.Context, room domain.Room) error {
	payload := messaging.RoomEventData{
		Room: room,
	}

	roomEventJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return p.rabbitmq.PublishMessage(ctx, contracts.EventRoomCreated, contracts.AmqpMessage{
		OwnerID: room.Owner.User.ID,
		Data:    roomEventJSON,
	})
}

func (p *RoomPublisher) PublishRoomDeleted(ctx context.Context, room domain.Room) error {
	payload := messaging.RoomEventData{
		Room: room,
	}

	roomEventJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return p.rabbitmq.PublishMessage(ctx, contracts.EventRoomDeleted, contracts.AmqpMessage{
		OwnerID: room.Owner.User.ID,
		Data:    roomEventJSON,
	})
}

func (p *RoomPublisher) PublishRoomJoined(ctx context.Context, room domain.Room) error {
	payload := messaging.RoomEventData{
		Room: room,
	}

	roomEventJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return p.rabbitmq.PublishMessage(ctx, contracts.EventMemberJoined, contracts.AmqpMessage{
		OwnerID: room.Owner.User.ID,
		Data:    roomEventJSON,
	})
}

func (p *RoomPublisher) PublishRoomLeave(ctx context.Context, room domain.Room) error {
	payload := messaging.RoomEventData{
		Room: room,
	}

	roomEventJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return p.rabbitmq.PublishMessage(ctx, contracts.EventMemberLeft, contracts.AmqpMessage{
		OwnerID: room.Owner.User.ID,
		Data:    roomEventJSON,
	})
}

func (p *RoomPublisher) PublishRoomMemberKicked(ctx context.Context, room domain.Room) error {
	payload := messaging.RoomEventData{
		Room: room,
	}

	roomEventJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return p.rabbitmq.PublishMessage(ctx, contracts.EventMemberKicked, contracts.AmqpMessage{
		OwnerID: room.Owner.User.ID,
		Data:    roomEventJSON,
	})
}
