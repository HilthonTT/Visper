package message

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/application/usecases/message"
	"github.com/hilthontt/visper/services/api/application/usecases/room"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/websocket"
	"github.com/hilthontt/visper/services/api/presentation/httpx"
)

// Default page sizes; the use case clamps the requested limit.
const (
	defaultMessagesLimit      = 50
	defaultMessagesAfterLimit = 100
)

type MessageController interface {
	SendMessage(ctx *gin.Context)
	UpdateMessage(ctx *gin.Context)
	DeleteMessage(ctx *gin.Context)
	GetMessages(ctx *gin.Context)
	GetMessagesAfter(ctx *gin.Context)
	GetMessageCount(ctx *gin.Context)
}

type messageController struct {
	usecase     message.MessageUseCase
	roomUseCase room.RoomUseCase
	wsCore      *websocket.Core
}

func NewMessageController(
	usecase message.MessageUseCase,
	roomUseCase room.RoomUseCase,
	wsCore *websocket.Core,
) MessageController {
	return &messageController{
		usecase:     usecase,
		roomUseCase: roomUseCase,
		wsCore:      wsCore,
	}
}

func (c *messageController) SendMessage(ctx *gin.Context) {
	var req SendMessageRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}
	roomID, user, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	msg, err := c.usecase.Send(ctx.Request.Context(), roomID, user.ID, user.Username, req.Content, req.Encrypted)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	c.wsCore.Publish(websocket.NewMessageReceived(
		roomID,
		msg.ID,
		msg.Content,
		msg.UserID,
		msg.Username,
		msg.CreatedAt.Format(time.RFC3339),
		msg.Encrypted,
	))

	ctx.JSON(http.StatusCreated, toMessageResponse(msg))
}

func (c *messageController) UpdateMessage(ctx *gin.Context) {
	messageID, ok := httpx.RequireParam(ctx, "messageId", "message ID")
	if !ok {
		return
	}
	var req UpdateMessageRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}
	roomID, user, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	if err := c.usecase.Update(ctx.Request.Context(), roomID, messageID, user.ID, req.Content, req.Encrypted); err != nil {
		httpx.Error(ctx, err)
		return
	}

	content := strings.TrimSpace(req.Content)
	c.wsCore.Publish(websocket.NewMessageUpdated(roomID, messageID, content, time.Now().Format(time.RFC3339), req.Encrypted))

	ctx.JSON(http.StatusOK, MessageUpdatedResponse{
		Success:   true,
		MessageID: messageID,
		Content:   content,
		Encrypted: req.Encrypted,
	})
}

func (c *messageController) DeleteMessage(ctx *gin.Context) {
	messageID, ok := httpx.RequireParam(ctx, "messageId", "message ID")
	if !ok {
		return
	}
	roomID, user, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	if err := c.usecase.Delete(ctx.Request.Context(), roomID, messageID, user.ID); err != nil {
		httpx.Error(ctx, err)
		return
	}

	c.wsCore.Publish(websocket.NewMessageDeleted(roomID, messageID, time.Now().Format(time.RFC3339)))

	ctx.JSON(http.StatusOK, MessageDeletedResponse{
		Success:   true,
		MessageID: messageID,
	})
}

func (c *messageController) GetMessages(ctx *gin.Context) {
	roomID, _, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	messages, err := c.usecase.GetRoomMessages(ctx.Request.Context(), roomID, parseLimit(ctx, defaultMessagesLimit))
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, toMessagesResponse(roomID, messages))
}

func (c *messageController) GetMessagesAfter(ctx *gin.Context) {
	timestamp, ok := parseTimestamp(ctx)
	if !ok {
		return
	}
	roomID, _, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	messages, err := c.usecase.GetMessagesAfter(ctx.Request.Context(), roomID, timestamp, parseLimit(ctx, defaultMessagesAfterLimit))
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, toMessagesResponse(roomID, messages))
}

func (c *messageController) GetMessageCount(ctx *gin.Context) {
	roomID, _, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	count, err := c.usecase.GetMessageCount(ctx.Request.Context(), roomID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, MessageCountResponse{RoomID: roomID, Count: count})
}

// requireRoomMember resolves the current user and checks that they are a
// member of the room in the ":id" path parameter. It writes the error
// response and returns ok=false when the request must not proceed.
func (c *messageController) requireRoomMember(ctx *gin.Context) (roomID string, user *model.User, ok bool) {
	if roomID, ok = httpx.RoomID(ctx); !ok {
		return "", nil, false
	}
	if user, ok = httpx.RequireUser(ctx); !ok {
		return "", nil, false
	}

	room, err := c.roomUseCase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		httpx.Error(ctx, err)
		return "", nil, false
	}
	if !room.IsMember(user.ID) {
		httpx.Fail(ctx, http.StatusForbidden, httpx.CodeForbidden, "you are not a member of this room")
		return "", nil, false
	}
	return roomID, user, true
}

// parseTimestamp reads the required RFC 3339 "timestamp" query parameter, or
// writes a 400 response.
func parseTimestamp(ctx *gin.Context) (time.Time, bool) {
	raw := ctx.Query("timestamp")
	if raw == "" {
		httpx.BadRequest(ctx, "timestamp parameter is required")
		return time.Time{}, false
	}
	timestamp, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpx.BadRequest(ctx, "invalid timestamp format, use RFC3339 (e.g., 2024-01-01T12:00:00Z)")
		return time.Time{}, false
	}
	return timestamp, true
}

// parseLimit reads the "limit" query parameter, falling back to def when it
// is missing or malformed. Range clamping is done by the use case.
func parseLimit(ctx *gin.Context, def int64) int64 {
	if parsed, err := strconv.ParseInt(ctx.Query("limit"), 10, 64); err == nil {
		return parsed
	}
	return def
}

func toMessageResponse(msg *model.Message) MessageResponse {
	return MessageResponse{
		ID:        msg.ID,
		RoomID:    msg.RoomID,
		UserID:    msg.UserID,
		Username:  msg.Username,
		Content:   msg.Content,
		CreatedAt: msg.CreatedAt,
		Encrypted: msg.Encrypted,
	}
}

func toMessagesResponse(roomID string, messages []*model.Message) MessagesResponse {
	responses := make([]MessageResponse, len(messages))
	for i, msg := range messages {
		responses[i] = toMessageResponse(msg)
	}
	return MessagesResponse{Messages: responses, Count: len(messages), RoomID: roomID}
}
