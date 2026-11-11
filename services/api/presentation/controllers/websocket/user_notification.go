package websocket

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	roomUseCase "github.com/hilthontt/visper/services/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/services/api/application/usecases/user"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/websocket"
	"github.com/hilthontt/visper/services/api/presentation/httpx"
)

// UserNotificationController serves the per-user notification stream, used to
// hand a room invite from one device (e.g. a scanned QR code) to another.
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
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	conn, err := c.wsNotificationCore.Upgrade(ctx.Writer, ctx.Request)
	if err != nil {
		// The upgrader has already written an HTTP error response.
		log.Printf("WebSocket upgrade failed for user %s: %v", user.ID, err)
		return
	}

	client := websocket.NewNotificationClient(conn, user.ID, user.Username)
	if !c.wsNotificationCore.RegisterClient(client) {
		_ = conn.Close()
		return
	}

	go client.WriteMessage()
	go client.ReadMessage(c.wsNotificationCore)
}

// NotifySelfRoomInvite sends a room invite to every notification stream of
// the user in the body, for a room identified by its join and secure codes.
func (c *userNotificationController) NotifySelfRoomInvite(ctx *gin.Context) {
	joinCode := ctx.Query("join_code")
	secureCode := ctx.Query("secure_code")
	if joinCode == "" || secureCode == "" {
		httpx.BadRequest(ctx, "join_code and secure_code query parameters are required")
		return
	}

	var req NotifySelfRoomInviteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(ctx, "user_id is required in request body")
		return
	}

	room, err := c.roomUseCase.GetByJoinCodeWithSecureToken(ctx.Request.Context(), joinCode, secureCode)
	if err != nil {
		// Deliberately vague: don't reveal which code was wrong.
		httpx.Fail(ctx, http.StatusNotFound, "room_not_found", "invalid join code or secure code")
		return
	}

	user, err := c.userUseCase.GetOrCreateUser(ctx.Request.Context(), req.UserID)
	if err != nil {
		httpx.Error(ctx, err)
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

	c.wsNotificationCore.NotifyUser(user.ID, websocket.NewNotificationMessage(
		"room_invite",
		user.ID,
		map[string]any{
			"room_id":     room.ID,
			"join_code":   joinCode,
			"secure_code": secureCode,
			"timestamp":   time.Now().Unix(),
			"expires_at":  formatExpiresAt(room),
		},
	))

	ctx.JSON(http.StatusOK, gin.H{
		"message":           "notification sent to your devices",
		"room_id":           room.ID,
		"user_id":           user.ID,
		"username":          user.Username,
		"notification_sent": true,
	})
}

// formatExpiresAt returns the room expiry as an RFC3339 timestamp (what
// clients parse), or an empty string for rooms that never expire.
func formatExpiresAt(room *model.Room) string {
	expiresAt := room.ExpiresAt()
	if expiresAt.IsZero() {
		return ""
	}
	return expiresAt.UTC().Format(time.RFC3339)
}
