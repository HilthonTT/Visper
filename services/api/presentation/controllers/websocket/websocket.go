package websocket

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/application/usecases/room"
	"github.com/hilthontt/visper/services/api/infrastructure/websocket"
	"github.com/hilthontt/visper/services/api/presentation/httpx"
)

// WebSocketController upgrades room members to the room's live event stream.
type WebSocketController interface {
	HandleConnection(ctx *gin.Context)
}

type webSocketController struct {
	roomUseCase   room.RoomUseCase
	wsRoomManager *websocket.RoomManager
	wsCore        *websocket.Core
}

func NewWebSocketController(
	roomUseCase room.RoomUseCase,
	wsRoomManager *websocket.RoomManager,
	wsCore *websocket.Core,
) WebSocketController {
	return &webSocketController{
		roomUseCase:   roomUseCase,
		wsRoomManager: wsRoomManager,
		wsCore:        wsCore,
	}
}

func (c *webSocketController) HandleConnection(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	room, err := c.roomUseCase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	if !room.IsMember(user.ID) {
		httpx.Fail(ctx, http.StatusForbidden, httpx.CodeForbidden, "you are not a member of this room")
		return
	}

	conn, err := c.wsRoomManager.Upgrade(ctx.Writer, ctx.Request)
	if err != nil {
		// The upgrader has already written an HTTP error response.
		log.Printf("WebSocket upgrade failed for user %s in room %s: %v", user.ID, roomID, err)
		return
	}

	client := websocket.NewClient(conn, user.ID, roomID, user.Username)
	if !c.wsCore.RegisterClient(client) {
		client.Close()
		return
	}

	c.wsCore.Publish(websocket.NewMemberJoined(roomID, websocket.MemberPayload{
		UserID:   user.ID,
		Username: user.Username,
		JoinedAt: time.Now().Format(time.RFC3339),
	}))

	go client.WriteMessage()
	go client.ReadMessage(c.wsCore)
}
