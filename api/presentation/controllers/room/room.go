package room

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/application/usecases/room"
	"github.com/hilthontt/visper/api/application/usecases/user"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"github.com/hilthontt/visper/api/infrastructure/security"
	"github.com/hilthontt/visper/api/infrastructure/websocket"
	"github.com/hilthontt/visper/api/presentation/middlewares"
)

type RoomController interface {
	GenerateNewJoinCode(ctx *gin.Context)
	RegenerateSecureToken(ctx *gin.Context)
	CreateRoom(ctx *gin.Context)
	GetRoom(ctx *gin.Context)
	JoinRoomByJoinCode(ctx *gin.Context)
	JoinRoomByJoinCodeWithToken(ctx *gin.Context)
	DeleteRoom(ctx *gin.Context)
	JoinRoom(ctx *gin.Context)
	LeaveRoom(ctx *gin.Context)
	CheckMembership(ctx *gin.Context)
	KickMember(ctx *gin.Context)
}

type roomController struct {
	usecase       room.RoomUseCase
	userUsecase   user.UserUseCase
	wsRoomManager *websocket.RoomManager
	wsCore        *websocket.Core
	config        *config.Config
}

func NewRoomController(
	usecase room.RoomUseCase,
	userUsecase user.UserUseCase,
	wsRoomManager *websocket.RoomManager,
	wsCore *websocket.Core,
	config *config.Config,
) RoomController {
	return &roomController{
		usecase:       usecase,
		userUsecase:   userUsecase,
		wsRoomManager: wsRoomManager,
		wsCore:        wsCore,
		config:        config,
	}
}

func (c *roomController) GenerateNewJoinCode(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	room, err := c.usecase.GenerateNewJoinCode(ctx.Request.Context(), user.ID, roomID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "only the room owner can update the room" {
			status = http.StatusForbidden
		} else if err.Error() == "room not found" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "update_failed",
			Message: err.Error(),
		})
		return
	}

	updatedMessage := websocket.NewRoomUpdated(room.ID, room.JoinCode)
	c.wsCore.Broadcast() <- updatedMessage

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "room join code regenerated successfully",
	})
}

func (c *roomController) CreateRoom(ctx *gin.Context) {
	var req CreateRoomRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: middlewares.TranslateValidationError(err),
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	expiry := time.Duration(req.ExpiryHrs) * time.Hour

	room, err := c.usecase.Create(ctx.Request.Context(), *user, expiry)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "creation_failed",
			Message: err.Error(),
		})
		return
	}

	if err := security.SetRoomAuth(ctx.Writer, user, room.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	ctx.JSON(http.StatusCreated, c.toRoomResponse(room, user))
}

func (c *roomController) GetRoom(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	room, err := c.usecase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "room not found" || err.Error() == "room has expired" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "not_found",
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, c.toRoomResponse(room, user))
}

func (c *roomController) GetRoomByJoinCode(ctx *gin.Context) {
	var req JoinByCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: middlewares.TranslateValidationError(err),
		})
		return
	}

	room, err := c.usecase.GetByJoinCode(ctx.Request.Context(), req.JoinCode)
	if err != nil {
		status := http.StatusNotFound
		if err.Error() != "room not found with join code: "+req.JoinCode &&
			err.Error() != "room has expired" {
			status = http.StatusInternalServerError
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "not_found",
			Message: err.Error(),
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}

	if err := c.usecase.JoinRoom(ctx.Request.Context(), room.ID, *user); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "join_failed",
			Message: err.Error(),
		})
		return
	}

	if err := security.SetRoomAuth(ctx.Writer, user, room.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	room, err = c.usecase.GetByID(ctx.Request.Context(), room.ID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "room not found" || err.Error() == "room has expired" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "not_found",
			Message: err.Error(),
		})
		return
	}

	c.wsCore.Broadcast() <- websocket.NewMemberJoined(room.ID, websocket.MemberPayload{
		UserID:   user.ID,
		Username: user.Username,
		JoinedAt: time.Now().Format(time.RFC3339),
	})

	ctx.JSON(http.StatusOK, c.toRoomResponse(room, user))
}

func (c *roomController) DeleteRoom(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	if err := c.usecase.Delete(ctx.Request.Context(), roomID, user.ID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "only the room owner can delete the room" {
			status = http.StatusForbidden
		} else if err.Error() == "room not found" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "deletion_failed",
			Message: err.Error(),
		})
		return
	}

	security.ClearRoomAuth(ctx.Writer, roomID)

	deleteMessage := websocket.NewRoomDeleted(roomID)
	c.wsCore.Broadcast() <- deleteMessage

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "room deleted successfully",
	})
}

func (c *roomController) JoinRoom(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	var req JoinRoomRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: middlewares.TranslateValidationError(err),
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}

	if err := c.usecase.JoinRoom(ctx.Request.Context(), roomID, *user); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "room not found" || err.Error() == "room has expired" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "join_failed",
			Message: err.Error(),
		})
		return
	}

	if err := security.SetRoomAuth(ctx.Writer, user, roomID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	joinMessage := websocket.NewMemberJoined(roomID, websocket.MemberPayload{
		UserID:   user.ID,
		Username: user.Username,
		JoinedAt: time.Now().Format(time.RFC3339),
	})
	c.wsCore.Broadcast() <- joinMessage

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "successfully joined room",
		Data: map[string]string{
			"room_id": roomID,
			"user_id": user.ID,
		},
	})
}

func (c *roomController) JoinRoomByJoinCode(ctx *gin.Context) {
	var req JoinByCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: middlewares.TranslateValidationError(err),
		})
		return
	}

	room, err := c.usecase.GetByJoinCode(ctx.Request.Context(), req.JoinCode)
	if err != nil {
		status := http.StatusNotFound
		if err.Error() != "room not found with join code: "+req.JoinCode &&
			err.Error() != "room has expired" {
			status = http.StatusInternalServerError
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "not_found",
			Message: err.Error(),
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}

	if err := c.usecase.JoinRoom(ctx.Request.Context(), room.ID, *user); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "room not found" || err.Error() == "room has expired" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "join_failed",
			Message: err.Error(),
		})
		return
	}

	if err := security.SetRoomAuth(ctx.Writer, user, room.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	joinMessage := websocket.NewMemberJoined(room.ID, websocket.MemberPayload{
		UserID:   user.ID,
		Username: user.Username,
		JoinedAt: time.Now().String(),
	})
	c.wsCore.Broadcast() <- joinMessage

	ctx.JSON(http.StatusOK, c.toRoomResponse(room, user))
}

func (c *roomController) LeaveRoom(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "room authentication required",
		})
		return
	}

	if err := c.usecase.LeaveRoom(ctx.Request.Context(), roomID, user.ID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "room not found" {
			status = http.StatusNotFound
		} else if err.Error() == "room owner cannot leave, delete the room instead" {
			status = http.StatusForbidden
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "leave_failed",
			Message: err.Error(),
		})
		return
	}

	security.ClearRoomAuth(ctx.Writer, roomID)

	leaveMessage := websocket.NewMemberLeft(roomID, user.ID, user.Username)
	c.wsCore.Broadcast() <- leaveMessage

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "successfully left room",
	})
}

func (c *roomController) CheckMembership(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusOK, map[string]any{
			"is_member": false,
			"room_id":   roomID,
		})
		return
	}

	isMember, err := c.usecase.IsUserInRoom(ctx.Request.Context(), roomID, user.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "check_failed",
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, map[string]any{
		"is_member": isMember,
		"room_id":   roomID,
		"user_id":   user.ID,
	})
}

func (c *roomController) KickMember(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	userToKickID := ctx.Param("userId")
	if userToKickID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "user ID is required",
		})
		return
	}

	userToKick, err := c.userUsecase.GetByID(ctx.Request.Context(), userToKickID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "user to kick not found",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusOK, map[string]any{
			"is_member": false,
			"room_id":   roomID,
		})
		return
	}

	if err := c.usecase.KickMember(ctx.Request.Context(), roomID, userToKickID, user.ID); err != nil {
		status := http.StatusInternalServerError
		errorCode := "kick_failed"

		switch {
		case err.Error() == "room not found":
			status = http.StatusNotFound
			errorCode = "not_found"
		case err.Error() == "only the room owner can kick members":
			status = http.StatusForbidden
			errorCode = "forbidden"
		case err.Error() == "room owner cannot be kicked, delete the room instead":
			status = http.StatusForbidden
			errorCode = "forbidden"
		case err.Error() == "user is not a member of this room":
			status = http.StatusNotFound
			errorCode = "not_found"
		}

		ctx.JSON(status, ErrorResponse{
			Error:   errorCode,
			Message: err.Error(),
		})
		return
	}

	const reason = "Removed by room owner"
	kickMessage := websocket.NewErrorKicked(roomID, userToKick.ID, userToKick.Username, reason)
	c.wsCore.Broadcast() <- kickMessage

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "member kicked successfully",
		Data: map[string]string{
			"room_id":        roomID,
			"kicked_user_id": userToKick.ID,
			"username":       userToKick.Username,
		},
	})
}

func (c *roomController) RegenerateSecureToken(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	room, err := c.usecase.RegenerateSecureCode(ctx.Request.Context(), user.ID, roomID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "only the room owner can update the room" {
			status = http.StatusForbidden
		} else if err.Error() == "room not found" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "update_failed",
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "secure token regenerated successfully",
		Data: map[string]string{
			"secure_code": room.SecureCode,
			"qr_code_url": room.GetQRCodeURL(c.config.GetServerAddress()),
		},
	})
}

func (c *roomController) JoinRoomByJoinCodeWithToken(ctx *gin.Context) {
	var req JoinByCodeWithTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: middlewares.TranslateValidationError(err),
		})
		return
	}

	room, err := c.usecase.GetByJoinCodeWithSecureToken(ctx.Request.Context(), req.JoinCode, req.SecureToken)
	if err != nil {
		status := http.StatusNotFound
		errorCode := "not_found"

		if err.Error() == "invalid secure token" {
			status = http.StatusForbidden
			errorCode = "invalid_token"
		} else if err.Error() != "room not found with join code: "+req.JoinCode &&
			err.Error() != "room has expired" {
			status = http.StatusInternalServerError
			errorCode = "server_error"
		}

		ctx.JSON(status, ErrorResponse{
			Error:   errorCode,
			Message: err.Error(),
		})
		return
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}

	if err := c.usecase.JoinRoom(ctx.Request.Context(), room.ID, *user); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "room not found" || err.Error() == "room has expired" {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "join_failed",
			Message: err.Error(),
		})
		return
	}

	if err := security.SetRoomAuth(ctx.Writer, user, room.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	joinMessage := websocket.NewMemberJoined(room.ID, websocket.MemberPayload{
		UserID:   user.ID,
		Username: user.Username,
		JoinedAt: time.Now().Format(time.RFC3339),
	})
	c.wsCore.Broadcast() <- joinMessage

	ctx.JSON(http.StatusOK, c.toRoomResponse(room, user))
}

func (c *roomController) toRoomResponse(room *model.Room, currentUser *model.User) RoomResponse {
	members := make([]UserResponse, len(room.Members))
	for i, member := range room.Members {
		members[i] = UserResponse{
			ID:       member.ID,
			Username: member.Username,
		}
	}

	expiresAt := room.CreatedAt.Add(room.Expiry)
	if room.Expiry == 0 {
		expiresAt = time.Time{} // Zero time if no expiry
	}

	return RoomResponse{
		ID:        room.ID,
		JoinCode:  room.JoinCode,
		QRCodeURL: room.GetQRCodeURL(c.config.GetServerAddress()),
		Owner: UserResponse{
			ID:       room.Owner.ID,
			Username: room.Owner.Username,
		},
		CreatedAt: room.CreatedAt,
		ExpiresAt: expiresAt,
		Members:   members,
		CurrentUser: UserResponse{
			ID:       currentUser.ID,
			Username: currentUser.Username,
		},
	}
}
