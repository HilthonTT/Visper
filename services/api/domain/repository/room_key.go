package repository

import (
	"context"

	"github.com/hilthontt/visper/services/api/domain/model"
)

// RoomKeyRepository stores what clients exchange to share a room's keys
// without the server learning them: members' key cards, the latest rekey,
// and who left since. Everything in it is opaque to the server.
type RoomKeyRepository interface {
	// AddCard stores a key card published by userID.
	AddCard(ctx context.Context, roomID, userID, card string) error
	// CardCount returns how many cards the room holds.
	CardCount(ctx context.Context, roomID string) (int64, error)
	// Cards returns each user's cards, oldest first.
	Cards(ctx context.Context, roomID string) (map[string][]string, error)

	// LatestRekey returns the room's latest rekey, or nil if it has none.
	LatestRekey(ctx context.Context, roomID string) (*model.Rekey, error)
	// SaveRekey stores rekey if its epoch follows the latest one, and clears
	// the departed members. Otherwise it reports false and the latest epoch.
	SaveRekey(ctx context.Context, roomID string, rekey *model.Rekey) (bool, int, error)

	// MarkDeparted records that userID left or was kicked.
	MarkDeparted(ctx context.Context, roomID, userID string) error
	// HasDeparted reports whether anyone left or was kicked since the
	// latest rekey.
	HasDeparted(ctx context.Context, roomID string) (bool, error)
}
