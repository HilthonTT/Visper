package websocket

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/api/application/usecases/user"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/security"
	"github.com/hilthontt/visper/api/infrastructure/websocket"
	"github.com/hilthontt/visper/api/presentation/middlewares"
)

type WebSocketController interface {
	HandleConnection(ctx *gin.Context)
}

type webSocketController struct {
	roomUseCase   room.RoomUseCase
	userUseCase   userUseCase.UserUseCase
	wsRoomManager *websocket.RoomManager
	wsCore        *websocket.Core
}

func NewWebSocketController(
	roomUseCase room.RoomUseCase,
	userUseCase userUseCase.UserUseCase,
	wsRoomManager *websocket.RoomManager,
	wsCore *websocket.Core,
) WebSocketController {
	return &webSocketController{
		roomUseCase:   roomUseCase,
		userUseCase:   userUseCase,
		wsRoomManager: wsRoomManager,
		wsCore:        wsCore,
	}
}

func (c *webSocketController) HandleConnection(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "room ID is required",
		})
		return
	}

	user, err := c.getUserFromRequest(ctx)
	if err != nil {
		log.Printf("Failed to authenticate user for WebSocket: %v", err)
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "authentication required - please provide X-User-ID header or valid cookies",
		})
		return
	}

	room, err := c.roomUseCase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		status := http.StatusNotFound
		if err.Error() != "room not found" && err.Error() != "room has expired" {
			status = http.StatusInternalServerError
		}
		ctx.JSON(status, gin.H{
			"error":   "room_error",
			"message": err.Error(),
		})
		return
	}

	if !room.IsMember(user.ID) {
		ctx.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "you are not a member of this room",
		})
		return
	}

	conn, err := c.wsRoomManager.Upgrade(ctx.Writer, ctx.Request)
	if err != nil {
		log.Printf("WebSocket upgrade failed for user %s in room %s: %v", user.ID, roomID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "upgrade_failed",
			"message": "failed to upgrade connection",
		})
		return
	}

	client := websocket.NewClient(conn, user.ID, roomID, user.Username)
	c.wsCore.Register() <- client

	joinMessage := websocket.NewMemberJoined(roomID, websocket.MemberPayload{
		UserID:   user.ID,
		Username: user.Username,
		JoinedAt: time.Now().String(),
	})
	c.wsCore.Broadcast() <- joinMessage

	go client.WriteMessage()
	go client.ReadMessage(c.wsCore)
}

func (c *webSocketController) getUserFromRequest(ctx *gin.Context) (*model.User, error) {

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
