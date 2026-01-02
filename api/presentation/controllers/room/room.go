package room

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/application/usecases/room"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/security"
)

type RoomController interface {
	CreateRoom(ctx *gin.Context)
	GetRoom(ctx *gin.Context)
	GetRoomByJoinCode(ctx *gin.Context)
	DeleteRoom(ctx *gin.Context)
	JoinRoom(ctx *gin.Context)
	LeaveRoom(ctx *gin.Context)
	CheckMembership(ctx *gin.Context)
}

type roomController struct {
	usecase room.RoomUseCase
}

func NewRoomController(usecase room.RoomUseCase) RoomController {
	return &roomController{
		usecase: usecase,
	}
}

func (c *roomController) CreateRoom(ctx *gin.Context) {
	var req CreateRoomRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	userID := security.GetOrCreateUserID(ctx.Writer, ctx.Request)

	owner := model.User{
		ID:        userID,
		Username:  req.Username,
		CreatedAt: time.Now(),
	}

	expiry := time.Duration(req.ExpiryHrs) * time.Hour

	room, err := c.usecase.Create(ctx.Request.Context(), owner, expiry)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "creation_failed",
			Message: err.Error(),
		})
		return
	}

	if err := security.SetRoomAuth(ctx.Writer, &owner, room.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	ctx.JSON(http.StatusCreated, c.toRoomResponse(room))
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

	ctx.JSON(http.StatusOK, c.toRoomResponse(room))
}

func (c *roomController) GetRoomByJoinCode(ctx *gin.Context) {
	var req JoinByCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
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

	userID := security.GetOrCreateUserID(ctx.Writer, ctx.Request)

	user := model.User{
		ID:        userID,
		Username:  req.Username,
		CreatedAt: time.Now(),
	}

	if err := c.usecase.JoinRoom(ctx.Request.Context(), room.ID, user); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "join_failed",
			Message: err.Error(),
		})
		return
	}

	if err := security.SetRoomAuth(ctx.Writer, &user, room.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	room, _ = c.usecase.GetByID(ctx.Request.Context(), room.ID)

	ctx.JSON(http.StatusOK, c.toRoomResponse(room))
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

	// Get authenticated user from room cookie
	user, err := security.GetRoomAuth(ctx.Request)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "room authentication required",
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

	// Clear room authentication cookies
	security.ClearRoomAuth(ctx.Writer, roomID)

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
			Message: err.Error(),
		})
		return
	}

	userID := security.GetOrCreateUserID(ctx.Writer, ctx.Request)

	user := model.User{
		ID:        userID,
		Username:  req.Username,
		CreatedAt: time.Now(),
	}

	if err := c.usecase.JoinRoom(ctx.Request.Context(), roomID, user); err != nil {
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

	// Set room authentication cookie
	if err := security.SetRoomAuth(ctx.Writer, &user, roomID); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "auth_failed",
			Message: "failed to set authentication",
		})
		return
	}

	ctx.JSON(http.StatusOK, SuccessResponse{
		Message: "successfully joined room",
		Data: map[string]string{
			"room_id": roomID,
			"user_id": user.ID,
		},
	})
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

	user, err := security.GetRoomAuth(ctx.Request)
	if err != nil {
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

	userID := security.GetUserID(ctx.Request)
	if userID == "" {
		ctx.JSON(http.StatusOK, map[string]any{
			"is_member": false,
			"room_id":   roomID,
		})
		return
	}

	isMember, err := c.usecase.IsUserInRoom(ctx.Request.Context(), roomID, userID)
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
		"user_id":   userID,
	})
}

func (c *roomController) toRoomResponse(room *model.Room) RoomResponse {
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
		ID:       room.ID,
		JoinCode: room.JoinCode,
		Owner: UserResponse{
			ID:       room.Owner.ID,
			Username: room.Owner.Username,
		},
		CreatedAt: room.CreatedAt,
		ExpiresAt: expiresAt,
		Members:   members,
	}
}
