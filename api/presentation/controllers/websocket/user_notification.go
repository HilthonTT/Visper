package websocket

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	roomUseCase "github.com/hilthontt/visper/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/api/application/usecases/user"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/security"
	"github.com/hilthontt/visper/api/infrastructure/websocket"
	"github.com/hilthontt/visper/api/presentation/middlewares"
)

type UserNotificationController interface {
	HandleUserNotificationConnection(ctx *gin.Context)
	NotifySelfRoomInvite(ctx *gin.Context)
}

type userNotificationController struct {
	userUseCase        userUseCase.UserUseCase
	roomUseCase        roomUseCase.RoomUseCase
	wsNotificationCore *websocket.NotificationCore
}

func NewUserNotificationController(
	userUseCase userUseCase.UserUseCase,
	roomUseCase roomUseCase.RoomUseCase,
	wsNotificationCore *websocket.NotificationCore,
) UserNotificationController {
	return &userNotificationController{
		userUseCase:        userUseCase,
		roomUseCase:        roomUseCase,
		wsNotificationCore: wsNotificationCore,
	}
}

func (c *userNotificationController) HandleUserNotificationConnection(ctx *gin.Context) {
	user, err := c.getUserFromRequest(ctx)
	if err != nil {
		log.Printf("Failed to authenticate user for notification WebSocket: %v", err)
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "authentication required",
		})
		return
	}

	conn, err := c.wsNotificationCore.Upgrade(ctx.Writer, ctx.Request)
	if err != nil {
		log.Printf("WebSocket upgrade failed for user %s: %v", user.ID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "upgrade_failed",
			"message": "failed to upgrade connection",
		})
		return
	}

	client := websocket.NewNotificationClient(conn, user.ID, user.Username)
	c.wsNotificationCore.Register() <- client

	log.Printf("User %s (%s) connected to notification stream", user.Username, user.ID)

	go client.WriteMessage()
	go client.ReadMessage(c.wsNotificationCore)
}

func (c *userNotificationController) NotifySelfRoomInvite(ctx *gin.Context) {
	joinCode := ctx.Query("join_code")
	secureCode := ctx.Query("secure_code")

	if joinCode == "" || secureCode == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "join_code and secure_code query parameters are required",
		})
		return
	}

	var req NotifySelfRoomInviteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "user_id is required in request body",
		})
		return
	}

	room, err := c.roomUseCase.GetByJoinCodeWithSecureToken(
		ctx.Request.Context(),
		joinCode,
		secureCode,
	)
	if err != nil {
		log.Printf("Failed to get room with join code %s: %v", joinCode, err)
		ctx.JSON(http.StatusNotFound, gin.H{
			"error":   "room_not_found",
			"message": "invalid join code or secure code",
		})
		return
	}

	user, err := c.userUseCase.GetOrCreateUser(ctx.Request.Context(), req.UserID)
	if err != nil {
		log.Printf("Failed to get/create user %s: %v", req.UserID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "user_error",
			"message": "failed to process user",
		})
		return
	}

	if room.IsMember(user.ID) {
		ctx.JSON(http.StatusOK, gin.H{
			"message":        "already_member",
			"room_id":        room.ID,
			"already_joined": true,
		})
		return
	}

	notification := websocket.NewNotificationMessage(
		"room_invite",
		user.ID,
		map[string]any{
			"room_id":     room.ID,
			"join_code":   joinCode,
			"secure_code": secureCode,
			"timestamp":   time.Now().Unix(),
			"expires_at":  room.Expiry.String(),
		},
	)

	c.wsNotificationCore.NotifyUser(user.ID, notification)
	log.Printf("Sent room invite notification to user %s for room %s", user.ID, room.ID)

	ctx.JSON(http.StatusOK, gin.H{
		"message":           "notification sent to your devices",
		"room_id":           room.ID,
		"user_id":           user.ID,
		"username":          user.Username,
		"notification_sent": true,
	})
}

func (c *userNotificationController) getUserFromRequest(ctx *gin.Context) (*model.User, error) {
	if user, exists := middlewares.GetUserFromContext(ctx); exists {
		log.Printf("User authenticated via middleware context: %s", user.ID)
		return user, nil
	}

	if user, err := security.GetRoomAuth(ctx.Request); err == nil {
		log.Printf("User authenticated via room auth cookie: %s", user.ID)
		return user, nil
	}

	if headerUserID := ctx.GetHeader("X-User-ID"); headerUserID != "" {
		user, err := c.userUseCase.GetOrCreateUser(ctx.Request.Context(), headerUserID)
		if err == nil {
			log.Printf("User authenticated via X-User-ID header: %s", user.ID)
			return user, nil
		}
		log.Printf("Failed to get user from X-User-ID header: %v", err)
	}

	return nil, fmt.Errorf("no valid authentication found")
}
