package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/cache"
	"github.com/redis/go-redis/v9"
)

// Room key data lives in Redis structures that are always read from Redis
// (sorted sets and sets), never from the local cache: a member's card or a
// rekey written through one API instance must be visible through the others
// at once.
//
//	room:<id>:cards     sorted set of {"u":user ID,"c":card}, by upload time
//	room:<id>:rekeys    sorted set holding the latest rekey, by epoch
//	room:<id>:departed  set of user IDs that left since the latest rekey
//
// roomRepository.Delete removes them with the room.

func roomCardsKey(roomID string) string    { return fmt.Sprintf("room:%s:cards", roomID) }
func roomRekeysKey(roomID string) string   { return fmt.Sprintf("room:%s:rekeys", roomID) }
func roomDepartedKey(roomID string) string { return fmt.Sprintf("room:%s:departed", roomID) }

type roomKeyRepository struct {
	cache *cache.DistributedCache
}

// NewRoomKeyRepository returns the Redis-backed RoomKeyRepository.
func NewRoomKeyRepository(cache *cache.DistributedCache) repository.RoomKeyRepository {
	return &roomKeyRepository{cache: cache}
}

type storedCard struct {
	UserID string `json:"u"`
	Card   string `json:"c"`
}

func (r *roomKeyRepository) AddCard(ctx context.Context, roomID, userID, card string) error {
	member, err := json.Marshal(storedCard{UserID: userID, Card: card})
	if err != nil {
		return err
	}
	return r.cache.ZAdd(ctx, roomCardsKey(roomID), redis.Z{
		Score:  float64(time.Now().UnixMicro()),
		Member: string(member),
	})
}

func (r *roomKeyRepository) CardCount(ctx context.Context, roomID string) (int64, error) {
	return r.cache.ZCard(ctx, roomCardsKey(roomID))
}

func (r *roomKeyRepository) Cards(ctx context.Context, roomID string) (map[string][]string, error) {
	members, err := r.cache.ZRange(ctx, roomCardsKey(roomID), 0, -1)
	if err != nil {
		return nil, err
	}
	cards := make(map[string][]string)
	for _, m := range members {
		var c storedCard
		if json.Unmarshal([]byte(m), &c) != nil || c.UserID == "" {
			continue
		}
		cards[c.UserID] = append(cards[c.UserID], c.Card)
	}
	return cards, nil
}

func (r *roomKeyRepository) LatestRekey(ctx context.Context, roomID string) (*model.Rekey, error) {
	members, err := r.cache.ZRevRange(ctx, roomRekeysKey(roomID), 0, 0)
	if err != nil || len(members) == 0 {
		return nil, err
	}
	var rekey model.Rekey
	if err := json.Unmarshal([]byte(members[0]), &rekey); err != nil {
		return nil, fmt.Errorf("decoding rekey: %w", err)
	}
	return &rekey, nil
}

func (r *roomKeyRepository) SaveRekey(ctx context.Context, roomID string, rekey *model.Rekey) (bool, int, error) {
	member, err := json.Marshal(rekey)
	if err != nil {
		return false, 0, err
	}
	ok, latest, err := r.cache.ZAppendNext(ctx, roomRekeysKey(roomID), string(member), int64(rekey.Epoch), roomDepartedKey(roomID))
	return ok, int(latest), err
}

func (r *roomKeyRepository) MarkDeparted(ctx context.Context, roomID, userID string) error {
	return r.cache.SAdd(ctx, roomDepartedKey(roomID), userID)
}

func (r *roomKeyRepository) HasDeparted(ctx context.Context, roomID string) (bool, error) {
	ids, err := r.cache.SMembers(ctx, roomDepartedKey(roomID))
	return len(ids) > 0, err
}
