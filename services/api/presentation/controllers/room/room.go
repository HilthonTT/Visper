package room

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/application/usecases/room"
	"github.com/hilthontt/visper/services/api/application/usecases/user"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/config"
	"github.com/hilthontt/visper/services/api/infrastructure/security"
	"github.com/hilthontt/visper/services/api/infrastructure/websocket"
	"github.com/hilthontt/visper/services/api/presentation/httpx"
)

// kickReason is sent to a member who was kicked.
const kickReason = "Removed by room owner"

type RoomController interface {
	CreateRoom(ctx *gin.Context)
	GetRoom(ctx *gin.Context)
	DeleteRoom(ctx *gin.Context)
	GenerateNewJoinCode(ctx *gin.Context)
	RegenerateSecureToken(ctx *gin.Context)

	JoinRoom(ctx *gin.Context)
	JoinRoomByJoinCode(ctx *gin.Context)
	JoinRoomByJoinCodeWithToken(ctx *gin.Context)
	LeaveRoom(ctx *gin.Context)
	CheckMembership(ctx *gin.Context)
	KickMember(ctx *gin.Context)
}

type roomController struct {
	usecase     room.RoomUseCase
	userUsecase user.UserUseCase
	wsCore      *websocket.Core
	config      *config.Config
}

func NewRoomController(
	usecase room.RoomUseCase,
	userUsecase user.UserUseCase,
	wsCore *websocket.Core,
	config *config.Config,
) RoomController {
	return &roomController{
		usecase:     usecase,
		userUsecase: userUsecase,
		wsCore:      wsCore,
		config:      config,
	}
}

// ---------------------------------------------------------------------------
// Rooms

func (c *roomController) CreateRoom(ctx *gin.Context) {
	var req CreateRoomRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	room, err := c.usecase.Create(ctx.Request.Context(), *user, time.Duration(req.ExpiryHrs)*time.Hour)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	if !setRoomAuth(ctx, user, room.ID) {
		return
	}

	ctx.JSON(http.StatusCreated, c.toRoomResponse(room, user))
}

func (c *roomController) GetRoom(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	room, err := c.usecase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	// The response carries the room's encryption key and join code, so only
	// members may read it.
	if !room.IsMember(user.ID) {
		httpx.Fail(ctx, http.StatusForbidden, httpx.CodeForbidden, "you are not a member of this room")
		return
	}

	ctx.JSON(http.StatusOK, c.toRoomResponse(room, user))
}

func (c *roomController) DeleteRoom(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	if err := c.usecase.Delete(ctx.Request.Context(), roomID, user.ID); err != nil {
		httpx.Error(ctx, err)
		return
	}

	security.ClearRoomAuth(ctx.Writer, roomID)
	c.wsCore.Publish(websocket.NewRoomDeleted(roomID))

	ctx.JSON(http.StatusOK, httpx.SuccessResponse{Message: "room deleted successfully"})
}

func (c *roomController) GenerateNewJoinCode(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	room, err := c.usecase.GenerateNewJoinCode(ctx.Request.Context(), user.ID, roomID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	c.wsCore.Publish(websocket.NewRoomUpdated(room.ID, room.JoinCode))

	ctx.JSON(http.StatusOK, httpx.SuccessResponse{Message: "room join code regenerated successfully"})
}

func (c *roomController) RegenerateSecureToken(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	room, err := c.usecase.RegenerateSecureCode(ctx.Request.Context(), user.ID, roomID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, httpx.SuccessResponse{
		Message: "secure token regenerated successfully",
		Data: map[string]string{
			"secure_code": room.SecureCode,
			"qr_code_url": room.GetQRCodeURL(c.config.GetFrontEndURL()),
		},
	})
}

// ---------------------------------------------------------------------------
// Membership

// JoinRoom joins a room by ID; the body must carry its join and secure codes.
func (c *roomController) JoinRoom(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	var req JoinRoomRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}
	user, ok := joiningUser(ctx, req.Username)
	if !ok {
		return
	}

	if err := c.usecase.JoinRoomWithCodes(ctx.Request.Context(), roomID, req.JoinCode, req.SecureToken, *user); err != nil {
		httpx.Error(ctx, err)
		return
	}
	if !c.completeJoin(ctx, user, roomID) {
		return
	}

	ctx.JSON(http.StatusOK, httpx.SuccessResponse{
		Message: "successfully joined room",
		Data: map[string]string{
			"room_id": roomID,
			"user_id": user.ID,
		},
	})
}

func (c *roomController) JoinRoomByJoinCode(ctx *gin.Context) {
	var req JoinByCodeRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}

	room, err := c.usecase.GetByJoinCode(ctx.Request.Context(), req.JoinCode)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	c.joinFoundRoom(ctx, room, req.Username)
}

func (c *roomController) JoinRoomByJoinCodeWithToken(ctx *gin.Context) {
	var req JoinByCodeWithTokenRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}

	room, err := c.usecase.GetByJoinCodeWithSecureToken(ctx.Request.Context(), req.JoinCode, req.SecureToken)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	c.joinFoundRoom(ctx, room, req.Username)
}

func (c *roomController) LeaveRoom(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	if err := c.usecase.LeaveRoom(ctx.Request.Context(), roomID, user.ID); err != nil {
		httpx.Error(ctx, err)
		return
	}

	security.ClearRoomAuth(ctx.Writer, roomID)
	c.wsCore.Publish(websocket.NewMemberLeft(roomID, user.ID, user.Username))

	ctx.JSON(http.StatusOK, httpx.SuccessResponse{Message: "successfully left room"})
}

func (c *roomController) CheckMembership(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	isMember, err := c.usecase.IsUserInRoom(ctx.Request.Context(), roomID, user.ID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, MembershipResponse{
		IsMember: isMember,
		RoomID:   roomID,
		UserID:   user.ID,
	})
}

func (c *roomController) KickMember(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	kickedID, ok := httpx.RequireParam(ctx, "userId", "user ID")
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	kicked, err := c.userUsecase.GetByID(ctx.Request.Context(), kickedID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	if err := c.usecase.KickMember(ctx.Request.Context(), roomID, kickedID, user.ID); err != nil {
		httpx.Error(ctx, err)
		return
	}

	c.wsCore.Publish(websocket.NewErrorKicked(roomID, kicked.ID, kicked.Username, kickReason))

	ctx.JSON(http.StatusOK, httpx.SuccessResponse{
		Message: "member kicked successfully",
		Data: map[string]string{
			"room_id":        roomID,
			"kicked_user_id": kicked.ID,
			"username":       kicked.Username,
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers

// joinFoundRoom joins a room found by its join code and responds with it.
func (c *roomController) joinFoundRoom(ctx *gin.Context, room *model.Room, username string) {
	user, ok := joiningUser(ctx, username)
	if !ok {
		return
	}
	if err := c.usecase.JoinRoom(ctx.Request.Context(), room.ID, *user); err != nil {
		httpx.Error(ctx, err)
		return
	}
	if !c.completeJoin(ctx, user, room.ID) {
		return
	}

	// Re-read the room so the response includes the new member.
	if updated, err := c.usecase.GetByID(ctx.Request.Context(), room.ID); err == nil {
		room = updated
	}
	ctx.JSON(http.StatusOK, c.toRoomResponse(room, user))
}

// joiningUser returns the current user, renamed to username if one was given.
func joiningUser(ctx *gin.Context, username string) (*model.User, bool) {
	user, ok := httpx.RequireUser(ctx)
	if ok && username != "" {
		user.Username = username
	}
	return user, ok
}

// completeJoin sets the room cookie and tells the room about its new member.
func (c *roomController) completeJoin(ctx *gin.Context, user *model.User, roomID string) bool {
	if !setRoomAuth(ctx, user, roomID) {
		return false
	}
	c.wsCore.Publish(websocket.NewMemberJoined(roomID, websocket.MemberPayload{
		UserID:   user.ID,
		Username: user.Username,
		JoinedAt: time.Now().Format(time.RFC3339),
	}))
	return true
}

// setRoomAuth sets the room's auth cookie, or writes a 500 response.
func setRoomAuth(ctx *gin.Context, user *model.User, roomID string) bool {
	if err := security.SetRoomAuth(ctx.Writer, user, roomID); err != nil {
		_ = ctx.Error(err)
		httpx.Fail(ctx, http.StatusInternalServerError, httpx.CodeInternal, "failed to set authentication")
		return false
	}
	return true
}

func (c *roomController) toRoomResponse(room *model.Room, currentUser *model.User) RoomResponse {
	members := make([]UserResponse, len(room.Members))
	for i, member := range room.Members {
		members[i] = toUserResponse(member)
	}

	return RoomResponse{
		ID:            room.ID,
		JoinCode:      room.JoinCode,
		QRCodeURL:     room.GetQRCodeURL(c.config.GetFrontEndURL()),
		Owner:         toUserResponse(room.Owner),
		CreatedAt:     room.CreatedAt,
		ExpiresAt:     room.ExpiresAt(),
		Members:       members,
		CurrentUser:   toUserResponse(*currentUser),
		EncryptionKey: room.EncryptionKey,
	}
}

func toUserResponse(u model.User) UserResponse {
	return UserResponse{ID: u.ID, Username: u.Username}
}
