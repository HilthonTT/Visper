package apisdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/hilthontt/visper/sdk/go/internal/apijson"
	"github.com/hilthontt/visper/sdk/go/internal/requestconfig"
	"github.com/hilthontt/visper/sdk/go/option"
)

type RoomService struct {
	Options []option.RequestOption
}

func NewRoomService(opts ...option.RequestOption) *RoomService {
	r := &RoomService{opts}
	return r
}

// Create creates a new room with specified expiry hours
func (r *RoomService) Create(ctx context.Context, body RoomCreateParams, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	path := "api/v1/rooms"

	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// Get retrieves a room by ID
func (r *RoomService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s", url.PathEscape(id))
	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

// Generates a new join code for the room
func (r *RoomService) GenerateNewJoinCode(ctx context.Context, id string, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/join-code", url.PathEscape(id))
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, nil, nil, opts...)

	return err
}

// Delete deletes a room (only owner can delete)
func (r *RoomService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s", url.PathEscape(id))
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)

	return err
}

// GetByJoinCode retrieves a room by join code and joins the user to it
func (r *RoomService) GetByJoinCode(ctx context.Context, body JoinByCodeParams, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if body.JoinCode == "" {
		return nil, ErrMissingJoinCodeParameter
	}

	path := "api/v1/rooms/join-code"
	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// JoinByCodeWithToken joins a room using its join code together with the
// secure token (as delivered in room invites and QR codes).
func (r *RoomService) JoinByCodeWithToken(ctx context.Context, body JoinByCodeWithTokenParams, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if body.JoinCode == "" {
		return nil, ErrMissingJoinCodeParameter
	}
	if body.SecureToken == "" {
		return nil, ErrMissingSecureTokenParameter
	}

	path := "api/v1/rooms/join-code/secure"
	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// Join joins an existing room by room ID
func (r *RoomService) Join(ctx context.Context, id string, body JoinRoomParams, opts ...option.RequestOption) (*SuccessResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/join", url.PathEscape(id))
	res := &SuccessResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// Leave leaves a room
func (r *RoomService) Leave(ctx context.Context, id string, opts ...option.RequestOption) (*SuccessResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/leave", url.PathEscape(id))
	res := &SuccessResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, nil, &res, opts...)

	return res, err
}

// CheckMembership checks if the current user is a member of a room
func (r *RoomService) CheckMembership(ctx context.Context, id string, opts ...option.RequestOption) (*MembershipResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/membership", url.PathEscape(id))
	res := &MembershipResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

// KickMember kicks a member from the room (only owner can kick)
func (r *RoomService) KickMember(ctx context.Context, roomID, userID string, opts ...option.RequestOption) (*SuccessResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	path := fmt.Sprintf("api/v1/rooms/%s/membership/%s", url.PathEscape(roomID), url.PathEscape(userID))
	res := &SuccessResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, nil, &res, opts...)

	return res, err
}

// Request/Response types
type RoomCreateParams struct {
	ExpiryHours int `json:"expiry_hours"` // 1 to 168 hours (1 hour to 7 days)
}

func (r *RoomCreateParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type JoinByCodeParams struct {
	JoinCode string `json:"join_code"` // 6-character join code
	Username string `json:"username,omitempty"`
}

func (r *JoinByCodeParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type JoinByCodeWithTokenParams struct {
	JoinCode    string `json:"join_code"`
	SecureToken string `json:"secure_token"`
	Username    string `json:"username,omitempty"`
}

func (r *JoinByCodeWithTokenParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

// JoinRoomParams are required to join by room ID: the room's join code and
// secure token, as for JoinByCodeWithToken.
type JoinRoomParams struct {
	JoinCode    string `json:"join_code"`
	SecureToken string `json:"secure_token"`
	Username    string `json:"username,omitempty"`
}

func (r *JoinRoomParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type RoomResponse struct {
	ID          string         `json:"id"`
	JoinCode    string         `json:"join_code"`
	QRCodeURL   string         `json:"qr_code_url"`
	Owner       UserResponse   `json:"owner"`
	CreatedAt   time.Time      `json:"created_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
	Members     []UserResponse `json:"members"`
	CurrentUser UserResponse   `json:"current_user"`
}

func (r *RoomResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type SuccessResponse struct {
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (r *SuccessResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MembershipResponse struct {
	IsMember bool   `json:"is_member"`
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id,omitempty"`
}

func (r *MembershipResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// ---------------------------------------------------------------------------
// Room keys (see RoomCrypto)

// CryptoStateResponse is what a member needs to verify the room's content
// and follow its key: every member's key cards, the latest rekey and whether
// one is due.
type CryptoStateResponse struct {
	// KeyEpoch is the epoch of the latest rekey (0 if none).
	KeyEpoch int `json:"key_epoch"`
	// RekeyNeeded is set when a member left or was kicked since the latest
	// rekey; the owner's client should rekey.
	RekeyNeeded bool `json:"rekey_needed"`
	// Rekey is the latest rekey, if any.
	Rekey *Rekey `json:"rekey"`
	// Cards holds each user's sealed key cards, oldest first, including
	// users who have since left (their messages are still in the history).
	Cards map[string][]string `json:"cards"`
	// Members lists the room's current members' user IDs.
	Members []string `json:"members"`
}

func (r *CryptoStateResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// GetCryptoState returns the room's key cards and latest rekey.
func (r *RoomService) GetCryptoState(ctx context.Context, roomID string, opts ...option.RequestOption) (*CryptoStateResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}
	path := fmt.Sprintf("api/v1/rooms/%s/crypto", url.PathEscape(roomID))
	res := &CryptoStateResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	return res, err
}

// PutKeyCardParams uploads the caller's key card (RoomCrypto.MyCard).
type PutKeyCardParams struct {
	Card string `json:"card"`
}

func (r *PutKeyCardParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

// PutKeyCard publishes the caller's key card to the room. The API relays it
// to the members as a MemberCard event.
func (r *RoomService) PutKeyCard(ctx context.Context, roomID string, body PutKeyCardParams, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if roomID == "" {
		return ErrMissingIDParameter
	}
	path := fmt.Sprintf("api/v1/rooms/%s/crypto/card", url.PathEscape(roomID))
	return requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, body, nil, opts...)
}

// PostRekey publishes a rekey made by RoomCrypto.NewRekey (owner only). The
// API refuses it unless its epoch follows the latest one; it relays accepted
// rekeys to the members as a RoomRekeyed event.
func (r *RoomService) PostRekey(ctx context.Context, roomID string, body Rekey, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if roomID == "" {
		return ErrMissingIDParameter
	}
	path := fmt.Sprintf("api/v1/rooms/%s/crypto/rekey", url.PathEscape(roomID))
	return requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, nil, opts...)
}

// PublishKeyCard uploads rc's card for the current key.
func (r *RoomService) PublishKeyCard(ctx context.Context, roomID string, rc *RoomCrypto, opts ...option.RequestOption) error {
	card, err := rc.MyCard()
	if err != nil {
		return err
	}
	if err := rc.AddCard(rc.Identity().UserID(), card); err != nil {
		return err
	}
	return r.PutKeyCard(ctx, roomID, PutKeyCardParams{Card: card}, opts...)
}

// SyncCrypto loads the room's key cards into rc and applies its latest
// rekey; if that changed the key, it uploads a new card. It returns the
// state it read.
func (r *RoomService) SyncCrypto(ctx context.Context, roomID string, rc *RoomCrypto, opts ...option.RequestOption) (*CryptoStateResponse, bool, error) {
	state, err := r.GetCryptoState(ctx, roomID, opts...)
	if err != nil {
		return nil, false, err
	}
	rc.AddCards(state.Cards)
	changed, err := rc.ApplyRekey(state.Rekey)
	if err != nil {
		return state, false, err
	}
	if changed {
		if err := r.PublishKeyCard(ctx, roomID, rc, opts...); err != nil {
			return state, true, err
		}
	}
	return state, changed, nil
}

// RotateKey makes and publishes a new room key for the room's members, as
// the owner, and switches rc to it. It returns the members who could not be
// given the key (no key card); they need a new invite.
func (r *RoomService) RotateKey(ctx context.Context, roomID string, rc *RoomCrypto, opts ...option.RequestOption) ([]string, error) {
	state, err := r.GetCryptoState(ctx, roomID, opts...)
	if err != nil {
		return nil, err
	}
	rc.AddCards(state.Cards)
	rekey, newKey, missing, err := rc.NewRekey(state.Members, state.KeyEpoch)
	if err != nil {
		return nil, err
	}
	if err := r.PostRekey(ctx, roomID, *rekey, opts...); err != nil {
		return nil, err
	}
	rc.CommitKey(newKey, rekey.Epoch)
	return missing, r.PublishKeyCard(ctx, roomID, rc, opts...)
}
