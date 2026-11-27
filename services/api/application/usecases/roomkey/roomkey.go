// Package roomkey relays what clients exchange to share and rotate a room's
// encryption key: members' key cards and the owner's rekeys. The server
// can't read or forge any of it (cards are sealed with the room key, rekeys
// are sealed to device keys and signed by the owner); it only checks who may
// publish what, and keeps the sizes bounded.
package roomkey

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hilthontt/visper/services/api/application/ports"
	"github.com/hilthontt/visper/services/api/application/usecases/room"
	"github.com/hilthontt/visper/services/api/domain/apperr"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
)

const (
	// MaxCardLength bounds a sealed key card (base64).
	MaxCardLength = 1024
	// MaxCardsPerRoom bounds the cards a room keeps. Members publish one
	// when they join and one per rekey.
	MaxCardsPerRoom = 1000
	// maxSealedKeyLength bounds one sealed key (base64url of 32 + 12 + 48).
	maxSealedKeyLength = 160
	// maxSignatureLength bounds a rekey signature (base64url of 64 bytes).
	maxSignatureLength = 128
)

var (
	base64Std = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)
	base64URL = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

	ErrInvalidCard    = apperr.Newf(apperr.Invalid, "a key card is base64, at most %d characters", MaxCardLength)
	ErrTooManyCards   = apperr.New(apperr.TooLarge, "this room holds too many key cards")
	ErrOwnerOnly      = apperr.New(apperr.Forbidden, "only the room owner can change the room key")
	ErrInvalidRekey   = apperr.New(apperr.Invalid, "invalid rekey")
	ErrRekeyNotMember = apperr.New(apperr.Invalid, "a rekey can only seal keys to the room's current members")
)

// State is a member's view of a room's keys.
type State struct {
	Epoch       int
	RekeyNeeded bool
	Rekey       *model.Rekey
	Cards       map[string][]string
	Members     []string
}

type UseCase interface {
	// State returns the room's key cards and latest rekey to one of its
	// members.
	State(ctx context.Context, roomID, userID string) (*State, error)
	// PutCard stores a key card published by userID, a member.
	PutCard(ctx context.Context, roomID, userID, card string) error
	// Rekey stores a rekey made by the room's owner.
	Rekey(ctx context.Context, roomID, userID string, rekey model.Rekey) (*model.Rekey, error)
	// MarkDeparted records that userID left or was kicked, so the owner's
	// client knows to rekey.
	MarkDeparted(ctx context.Context, roomID, userID string) error
}

type useCase struct {
	rooms    room.RoomUseCase
	repo     repository.RoomKeyRepository
	notifier ports.Notifier
}

// New creates the room key use case; notifier may be nil.
func New(rooms room.RoomUseCase, repo repository.RoomKeyRepository, notifier ports.Notifier) UseCase {
	if notifier == nil {
		notifier = ports.NopNotifier{}
	}
	return &useCase{rooms: rooms, repo: repo, notifier: notifier}
}

// memberRoom returns the room if userID is a member.
func (uc *useCase) memberRoom(ctx context.Context, roomID, userID string) (*model.Room, error) {
	r, err := uc.rooms.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !r.IsMember(userID) {
		return nil, room.ErrNotMember
	}
	return r, nil
}

func (uc *useCase) State(ctx context.Context, roomID, userID string) (*State, error) {
	r, err := uc.memberRoom(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	cards, err := uc.repo.Cards(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to read key cards: %w", err)
	}
	rekey, err := uc.repo.LatestRekey(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to read rekey: %w", err)
	}
	departed, err := uc.repo.HasDeparted(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to read departures: %w", err)
	}

	state := &State{Rekey: rekey, Cards: cards, RekeyNeeded: departed, Members: memberIDs(r)}
	if rekey != nil {
		state.Epoch = rekey.Epoch
	}
	return state, nil
}

func (uc *useCase) PutCard(ctx context.Context, roomID, userID, card string) error {
	if len(card) == 0 || len(card) > MaxCardLength || !base64Std.MatchString(card) {
		return ErrInvalidCard
	}
	if _, err := uc.memberRoom(ctx, roomID, userID); err != nil {
		return err
	}
	n, err := uc.repo.CardCount(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to count key cards: %w", err)
	}
	if n >= MaxCardsPerRoom {
		return ErrTooManyCards
	}
	if err := uc.repo.AddCard(ctx, roomID, userID, card); err != nil {
		return fmt.Errorf("failed to store key card: %w", err)
	}
	uc.notifier.MemberCard(roomID, userID, card)
	return nil
}

func (uc *useCase) Rekey(ctx context.Context, roomID, userID string, rekey model.Rekey) (*model.Rekey, error) {
	r, err := uc.memberRoom(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if r.Owner.ID != userID {
		return nil, ErrOwnerOnly
	}
	if rekey.Epoch < 1 || len(rekey.Signature) > maxSignatureLength || !base64URL.MatchString(rekey.Signature) {
		return nil, ErrInvalidRekey
	}
	for id, sealed := range rekey.Keys {
		if id == r.Owner.ID || !r.IsMember(id) {
			return nil, ErrRekeyNotMember
		}
		if len(sealed) > maxSealedKeyLength || !base64URL.MatchString(sealed) {
			return nil, ErrInvalidRekey
		}
	}
	if rekey.Keys == nil {
		rekey.Keys = map[string]string{}
	}

	ok, latest, err := uc.repo.SaveRekey(ctx, roomID, &rekey)
	if err != nil {
		return nil, fmt.Errorf("failed to store rekey: %w", err)
	}
	if !ok {
		return nil, apperr.Newf(apperr.Conflict, "the room key changed meanwhile: the next epoch is %d", latest+1)
	}
	uc.notifier.RoomRekeyed(roomID, &rekey)
	return &rekey, nil
}

func (uc *useCase) MarkDeparted(ctx context.Context, roomID, userID string) error {
	return uc.repo.MarkDeparted(ctx, roomID, userID)
}

func memberIDs(r *model.Room) []string {
	ids := make([]string, 0, len(r.Members)+1)
	seen := map[string]bool{}
	for _, u := range append([]model.User{r.Owner}, r.Members...) {
		if u.ID != "" && !seen[u.ID] {
			seen[u.ID] = true
			ids = append(ids, u.ID)
		}
	}
	return ids
}
