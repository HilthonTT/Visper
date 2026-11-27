package room

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/services/api/application/ports"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"go.uber.org/zap"
)

type RoomUseCase interface {
	Create(ctx context.Context, owner model.User, expiry time.Duration) (*model.Room, error)
	// GetByID returns a room that exists and has not expired.
	GetByID(ctx context.Context, id string) (*model.Room, error)
	GetByJoinCode(ctx context.Context, joinCode string) (*model.Room, error)
	GetByJoinCodeWithSecureToken(ctx context.Context, joinCode, secureCode string) (*model.Room, error)
	Delete(ctx context.Context, id, userID string) error
	GenerateNewJoinCode(ctx context.Context, userID, id string) (*model.Room, error)
	RegenerateSecureCode(ctx context.Context, userID, id string) (*model.Room, error)

	// JoinRoom adds user to the room if it still has the codes they
	// presented (an empty code is not checked; only the owner joins with
	// none, when creating the room).
	JoinRoom(ctx context.Context, roomID string, user model.User, codes repository.RoomCodes) error
	JoinRoomWithCodes(ctx context.Context, roomID, joinCode, secureCode string, user model.User) error
	LeaveRoom(ctx context.Context, roomID, userID string) error
	// KickMember removes a member and rotates the room's join and secure
	// codes, so the kicked user can't rejoin with the codes they already saw.
	// It returns the updated room and the kicked member.
	KickMember(ctx context.Context, roomID, userID, requesterID string) (*model.Room, model.User, error)
	IsUserInRoom(ctx context.Context, roomID, userID string) (bool, error)

	// ExpireRooms deletes every room whose expiry has passed and returns how many it removed.
	ExpireRooms(ctx context.Context) (int, error)
}

// kickReason is sent to a member who was kicked.
const kickReason = "Removed by room owner"

type roomUseCase struct {
	repository repository.RoomRepository
	keys       repository.RoomKeyRepository
	notifier   ports.Notifier
	events     *ports.AsyncEvents
	logger     ports.Logger
}

// NewRoomUseCase creates the room use case. keys records departures so the
// owner's client knows to rekey; it, notifier and events may be nil.
func NewRoomUseCase(
	repository repository.RoomRepository,
	keys repository.RoomKeyRepository,
	notifier ports.Notifier,
	events *ports.AsyncEvents,
	logger ports.Logger,
) RoomUseCase {
	if notifier == nil {
		notifier = ports.NopNotifier{}
	}
	return &roomUseCase{
		repository: repository,
		keys:       keys,
		notifier:   notifier,
		events:     events,
		logger:     logger,
	}
}

// ---------------------------------------------------------------------------
// Rooms

func (uc *roomUseCase) Create(ctx context.Context, owner model.User, expiry time.Duration) (*model.Room, error) {
	// Rooms have no server-side key: clients create the room key and share
	// it in invites, so the server only ever stores ciphertext.
	room := &model.Room{
		ID:         uuid.NewString(),
		JoinCode:   generateJoinCode(),
		Owner:      owner,
		CreatedAt:  time.Now(),
		Expiry:     expiry,
		Members:    []model.User{owner}, // the owner is a member too
		SecureCode: generateSecureCode(),
	}

	if err := uc.repository.Create(ctx, room); err != nil {
		uc.logger.Error("failed to create room", zap.Error(err), zap.String("ownerID", owner.ID))
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	if err := uc.repository.AddUser(ctx, room.ID, owner, repository.RoomCodes{}); err != nil {
		uc.logger.Error("failed to add owner to room", zap.Error(err), zap.String("roomID", room.ID), zap.String("ownerID", owner.ID))
		_ = uc.repository.Delete(ctx, room.ID)
		return nil, fmt.Errorf("failed to add owner to room: %w", err)
	}

	uc.events.Publish("room created", func(p ports.EventPublisher) error {
		return p.PublishRoomCreated(room.ID, owner.ID, room.Expiry)
	})

	uc.logger.Info("room created successfully", zap.String("roomID", room.ID))
	return room, nil
}

func (uc *roomUseCase) GetByID(ctx context.Context, id string) (*model.Room, error) {
	if id == "" {
		return nil, ErrRoomIDRequired
	}
	room, err := uc.loadRoom(ctx, id)
	if err != nil {
		return nil, err
	}
	if room.HasExpired() {
		_ = uc.deleteExpired(ctx, room)
		return nil, ErrRoomExpired
	}
	return room, nil
}

func (uc *roomUseCase) GetByJoinCode(ctx context.Context, joinCode string) (*model.Room, error) {
	if joinCode == "" {
		return nil, ErrJoinCodeRequired
	}
	return uc.findByJoinCode(ctx, joinCode)
}

func (uc *roomUseCase) GetByJoinCodeWithSecureToken(ctx context.Context, joinCode, secureCode string) (*model.Room, error) {
	if joinCode == "" {
		return nil, ErrJoinCodeRequired
	}
	if secureCode == "" {
		return nil, ErrSecureTokenRequired
	}

	room, err := uc.findByJoinCode(ctx, joinCode)
	if err != nil {
		return nil, err
	}
	if !codesEqual(room.SecureCode, secureCode) {
		// The join code is a credential: log the room, not the code.
		uc.logger.Warn("invalid secure token provided", zap.String("roomID", room.ID))
		return nil, ErrInvalidSecureToken
	}
	return room, nil
}

func (uc *roomUseCase) Delete(ctx context.Context, id, userID string) error {
	room, err := uc.loadOwnedRoom(ctx, id, userID, ErrOwnerOnlyDelete)
	if err != nil {
		return err
	}

	if err := uc.repository.Delete(ctx, room.ID); err != nil {
		uc.logger.Error("failed to delete room", zap.Error(err), zap.String("roomID", id))
		return fmt.Errorf("failed to delete room: %w", err)
	}

	uc.notifier.RoomDeleted(room.ID)
	uc.logger.Info("room deleted successfully", zap.String("roomID", id), zap.String("ownerID", userID))
	return nil
}

func (uc *roomUseCase) GenerateNewJoinCode(ctx context.Context, userID, id string) (*model.Room, error) {
	room, err := uc.updateOwnedRoom(ctx, id, userID, func(room *model.Room) {
		room.JoinCode = generateJoinCode()
	})
	if err == nil {
		uc.notifier.RoomUpdated(room.ID, room.JoinCode)
	}
	return room, err
}

func (uc *roomUseCase) RegenerateSecureCode(ctx context.Context, userID, id string) (*model.Room, error) {
	room, err := uc.updateOwnedRoom(ctx, id, userID, func(room *model.Room) {
		room.SecureCode = generateSecureCode()
	})
	if err == nil {
		uc.logger.Info("secure code regenerated", zap.String("roomID", id), zap.String("ownerID", userID))
	}
	return room, err
}

// ---------------------------------------------------------------------------
// Membership

func (uc *roomUseCase) JoinRoom(ctx context.Context, roomID string, user model.User, codes repository.RoomCodes) error {
	room, err := uc.GetByID(ctx, roomID)
	if err != nil {
		return err
	}

	if room.IsMember(user.ID) {
		// Announced again: the member may be joining from another client.
		uc.logger.Debug("user already in room", zap.String("roomID", roomID), zap.String("userID", user.ID))
		uc.notifier.MemberJoined(roomID, user)
		return nil
	}

	if err := uc.repository.AddUser(ctx, roomID, user, codes); err != nil {
		switch {
		case errors.Is(err, repository.ErrCodesChanged):
			// The codes were rotated (e.g. by a kick) after the room was read.
			uc.logger.Warn("join with codes that were just rotated", zap.String("roomID", roomID), zap.String("userID", user.ID))
			return ErrInvalidCodes
		case errors.Is(err, repository.ErrNotFound):
			return ErrRoomNotFound
		}
		uc.logger.Error("failed to add user to room", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", user.ID))
		return fmt.Errorf("failed to join room: %w", err)
	}

	uc.events.Publish("room joined", func(p ports.EventPublisher) error {
		return p.PublishRoomJoined(room.ID, user.ID)
	})

	uc.notifier.MemberJoined(roomID, user)
	uc.logger.Info("user joined room", zap.String("roomID", roomID), zap.String("userID", user.ID), zap.String("username", user.Username))
	return nil
}

// JoinRoomWithCodes joins a room by ID, but only for callers who know both
// its join code and its secure code. A room ID alone is not a credential:
// it appears in URLs, logs and events.
func (uc *roomUseCase) JoinRoomWithCodes(ctx context.Context, roomID, joinCode, secureCode string, user model.User) error {
	if joinCode == "" || secureCode == "" {
		return ErrInvalidCodes
	}

	room, err := uc.GetByID(ctx, roomID)
	if err != nil {
		return err
	}

	// Check both before failing so the response does not reveal which one was wrong.
	joinOK := codesEqual(room.JoinCode, joinCode)
	secureOK := codesEqual(room.SecureCode, secureCode)
	if !joinOK || !secureOK {
		uc.logger.Warn("join by room ID with invalid codes", zap.String("roomID", roomID), zap.String("userID", user.ID))
		return ErrInvalidCodes
	}

	return uc.JoinRoom(ctx, roomID, user, repository.RoomCodes{JoinCode: joinCode, SecureCode: secureCode})
}

func (uc *roomUseCase) LeaveRoom(ctx context.Context, roomID, userID string) error {
	if userID == "" {
		return ErrUserIDRequired
	}
	room, err := uc.GetByID(ctx, roomID)
	if err != nil {
		return err
	}

	if room.Owner.ID == userID {
		return ErrOwnerCannotLeave
	}
	// Without this check any user could "leave" any room, which broadcasts a
	// bogus member-left notice to its members.
	member, ok := findMember(room, userID)
	if !ok {
		return ErrNotMember
	}

	if err := uc.repository.RemoveUser(ctx, roomID, userID); err != nil {
		uc.logger.Error("failed to remove user from room", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", userID))
		return fmt.Errorf("failed to leave room: %w", err)
	}
	uc.markDeparted(ctx, roomID, userID)

	uc.notifier.MemberLeft(roomID, member)
	uc.logger.Info("user left room", zap.String("roomID", roomID), zap.String("userID", userID))
	return nil
}

func (uc *roomUseCase) KickMember(ctx context.Context, roomID, userID, requesterID string) (*model.Room, model.User, error) {
	if userID == "" {
		return nil, model.User{}, ErrUserIDRequired
	}
	room, err := uc.loadOwnedRoom(ctx, roomID, requesterID, ErrOwnerOnlyKick)
	if err != nil {
		return nil, model.User{}, err
	}

	if userID == room.Owner.ID {
		return nil, model.User{}, ErrOwnerCannotBeKicked
	}
	kicked, ok := findMember(room, userID)
	if !ok {
		return nil, model.User{}, ErrNotMember
	}

	// Every member can see the join code, and joining by join code alone is
	// allowed, so without new codes a kicked user could rejoin at once and be
	// sealed into the next room key.
	//
	// The codes change before the member is removed. If removal then fails
	// the user is still a member, so the owner can simply retry the kick;
	// the other way round, a failed rotation would leave the user out of the
	// room's member set (so a retry is refused as "not a member") yet still
	// connected and still able to rejoin with the old codes. And AddUser
	// checks the codes atomically, so a rejoin racing this kick either lands
	// before the rotation (and is undone by the removal below) or fails.
	room.JoinCode = generateJoinCode()
	room.SecureCode = generateSecureCode()
	if err := uc.repository.Update(ctx, room); err != nil {
		uc.logger.Error("failed to rotate codes before kick", zap.Error(err), zap.String("roomID", roomID))
		return nil, model.User{}, fmt.Errorf("failed to rotate room codes: %w", err)
	}

	if err := uc.repository.RemoveUser(ctx, roomID, userID); err != nil {
		uc.logger.Error("failed to kick user from room", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", userID))
		return nil, model.User{}, fmt.Errorf("failed to kick member: %w", err)
	}
	room.Members = slices.DeleteFunc(room.Members, func(u model.User) bool { return u.ID == userID })
	uc.markDeparted(ctx, roomID, userID)

	uc.notifier.MemberKicked(roomID, kicked, kickReason)
	// Sent after the kick, which disconnects the kicked user first, so only
	// the remaining members learn the new join code.
	uc.notifier.RoomUpdated(room.ID, room.JoinCode)

	uc.logger.Info("user kicked from room", zap.String("roomID", roomID), zap.String("kickedUserID", userID), zap.String("kickedBy", requesterID))
	return room, kicked, nil
}

func (uc *roomUseCase) IsUserInRoom(ctx context.Context, roomID, userID string) (bool, error) {
	if roomID == "" {
		return false, ErrRoomIDRequired
	}
	if userID == "" {
		return false, ErrUserIDRequired
	}

	userIDs, err := uc.repository.GetUsers(ctx, roomID)
	if err != nil {
		uc.logger.Error("failed to get room users", zap.Error(err), zap.String("roomID", roomID))
		return false, fmt.Errorf("failed to check room membership: %w", err)
	}

	return slices.Contains(userIDs, userID), nil
}

// ---------------------------------------------------------------------------
// Expiry

func (uc *roomUseCase) ExpireRooms(ctx context.Context) (int, error) {
	rooms, err := uc.repository.GetAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list rooms: %w", err)
	}

	expired := 0
	for _, room := range rooms {
		if room.HasExpired() && uc.deleteExpired(ctx, room) == nil {
			expired++
		}
	}
	return expired, nil
}

// deleteExpired deletes an expired room and tells its clients, so that its
// live websocket connections are closed. Without that,
// clients stayed connected to (and kept chatting in) a room that no longer
// existed.
func (uc *roomUseCase) deleteExpired(ctx context.Context, room *model.Room) error {
	uc.logger.Info("room has expired, deleting", zap.String("roomID", room.ID))
	if err := uc.repository.Delete(ctx, room.ID); err != nil {
		uc.logger.Error("failed to delete expired room", zap.Error(err), zap.String("roomID", room.ID))
		return err
	}
	uc.notifier.RoomDeleted(room.ID)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers

// loadRoom reads a room without checking its expiry.
func (uc *roomUseCase) loadRoom(ctx context.Context, id string) (*model.Room, error) {
	room, err := uc.repository.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && room == nil) {
		return nil, ErrRoomNotFound
	}
	if err != nil {
		uc.logger.Error("failed to get room", zap.Error(err), zap.String("roomID", id))
		return nil, fmt.Errorf("failed to get room: %w", err)
	}
	return room, nil
}

// loadOwnedRoom loads an unexpired room that userID must own; notOwner is
// returned when they don't.
func (uc *roomUseCase) loadOwnedRoom(ctx context.Context, id, userID string, notOwner error) (*model.Room, error) {
	room, err := uc.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if room.Owner.ID != userID {
		uc.logger.Warn("room owner action attempted by another user",
			zap.String("roomID", id), zap.String("userID", userID), zap.String("ownerID", room.Owner.ID), zap.Error(notOwner))
		return nil, notOwner
	}
	return room, nil
}

// updateOwnedRoom applies change to a room owned by userID and saves it.
func (uc *roomUseCase) updateOwnedRoom(ctx context.Context, id, userID string, change func(*model.Room)) (*model.Room, error) {
	room, err := uc.loadOwnedRoom(ctx, id, userID, ErrOwnerOnlyUpdate)
	if err != nil {
		return nil, err
	}
	change(room)
	if err := uc.repository.Update(ctx, room); err != nil {
		uc.logger.Error("failed to update room", zap.Error(err), zap.String("roomID", id))
		return nil, fmt.Errorf("failed to update room: %w", err)
	}
	return room, nil
}

// findByJoinCode returns the unexpired room using joinCode.
func (uc *roomUseCase) findByJoinCode(ctx context.Context, joinCode string) (*model.Room, error) {
	room, err := uc.repository.GetByJoinCode(ctx, joinCode)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, errJoinCodeNotFound(joinCode)
	}
	if err != nil {
		uc.logger.Error("failed to find room by join code", zap.Error(err))
		return nil, fmt.Errorf("failed to search for room: %w", err)
	}
	if room.HasExpired() {
		_ = uc.deleteExpired(ctx, room)
		return nil, ErrRoomExpired
	}
	return room, nil
}

// markDeparted records that userID left, so the owner's client knows a
// rekey is due. The member is out either way, so a failure is only logged.
func (uc *roomUseCase) markDeparted(ctx context.Context, roomID, userID string) {
	if uc.keys == nil {
		return
	}
	if err := uc.keys.MarkDeparted(ctx, roomID, userID); err != nil {
		uc.logger.Error("failed to mark member departed", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", userID))
	}
}

// findMember returns the member of room with userID (the owner included).
func findMember(room *model.Room, userID string) (model.User, bool) {
	if userID == "" {
		return model.User{}, false
	}
	if room.Owner.ID == userID {
		return room.Owner, true
	}
	i := slices.IndexFunc(room.Members, func(u model.User) bool { return u.ID == userID })
	if i < 0 {
		return model.User{}, false
	}
	return room.Members[i], true
}

// codesEqual compares room secrets in constant time.
func codesEqual(expected, provided string) bool {
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}
