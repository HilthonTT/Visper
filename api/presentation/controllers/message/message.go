package message

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/application/usecases/message"
	"github.com/hilthontt/visper/api/application/usecases/room"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/infrastructure/websocket"
	"github.com/hilthontt/visper/api/presentation/middlewares"
)

type MessageController interface {
	UpdateMessage(ctx *gin.Context)
	DeleteMessage(ctx *gin.Context)
	SendMessage(ctx *gin.Context)
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

// requireRoomMember resolves the current user and checks that they are a
// member of the room in the ":id" path parameter. It writes the error
// response and returns ok=false when the request must not proceed.
func (c *messageController) requireRoomMember(ctx *gin.Context) (roomID string, user *model.User, ok bool) {
	roomID = ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return "", nil, false
	}

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return "", nil, false
	}

	room, err := c.roomUseCase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "room not found",
		})
		return "", nil, false
	}

	if !room.IsMember(user.ID) {
		ctx.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "you are not a member of this room",
		})
		return "", nil, false
	}

	return roomID, user, true
}

func (c *messageController) DeleteMessage(ctx *gin.Context) {
	messageID := ctx.Param("messageId")
	if messageID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "message ID is required",
		})
		return
	}

	roomID, user, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	if err := c.usecase.Delete(ctx.Request.Context(), roomID, messageID, user.ID); err != nil {
		status := http.StatusInternalServerError
		errorCode := "delete_failed"

		switch {
		case strings.HasPrefix(err.Error(), "message not found"):
			status = http.StatusNotFound
			errorCode = "not_found"
		case err.Error() == "unauthorized: you can only delete your own messages":
			status = http.StatusForbidden
			errorCode = "forbidden"
		}

		ctx.JSON(status, ErrorResponse{
			Error:   errorCode,
			Message: err.Error(),
		})
		return
	}

	c.wsCore.Publish(websocket.NewMessageDeleted(roomID, messageID, time.Now().Format(time.RFC3339)))

	ctx.JSON(http.StatusOK, MessageDeletedResponse{
		Success:   true,
		MessageID: messageID,
	})
}

func (c *messageController) UpdateMessage(ctx *gin.Context) {
	messageID := ctx.Param("messageId")
	if messageID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "message ID is required",
		})
		return
	}

	var req UpdateMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: middlewares.TranslateValidationError(err),
		})
		return
	}

	roomID, user, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	if err := c.usecase.Update(ctx.Request.Context(), roomID, messageID, user.ID, req.Content, req.Encrypted); err != nil {
		status := http.StatusInternalServerError
		errorCode := "update_failed"

		switch {
		case strings.HasPrefix(err.Error(), "message not found"):
			status = http.StatusNotFound
			errorCode = "not_found"
		case err.Error() == "unauthorized: you can only edit your own messages":
			status = http.StatusForbidden
			errorCode = "forbidden"
		case isContentValidationError(err):
			status = http.StatusBadRequest
			errorCode = "invalid_content"
		}

		ctx.JSON(status, ErrorResponse{
			Error:   errorCode,
			Message: err.Error(),
		})
		return
	}

	content := strings.TrimSpace(req.Content)
	c.wsCore.Publish(websocket.NewMessageUpdated(
		roomID,
		messageID,
		content,
		time.Now().Format(time.RFC3339),
		req.Encrypted,
	))

	ctx.JSON(http.StatusOK, MessageUpdatedResponse{
		Success:   true,
		MessageID: messageID,
		Content:   content,
		Encrypted: req.Encrypted,
	})
}

func (c *messageController) SendMessage(ctx *gin.Context) {
	var req SendMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: middlewares.TranslateValidationError(err),
		})
		return
	}

	roomID, user, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	msg, err := c.usecase.Send(ctx.Request.Context(), roomID, user.ID, user.Username, req.Content, req.Encrypted)
	if err != nil {
		status := http.StatusInternalServerError
		if isContentValidationError(err) {
			status = http.StatusBadRequest
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "send_failed",
			Message: err.Error(),
		})
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

	ctx.JSON(http.StatusCreated, c.toMessageResponse(msg))
}

func (c *messageController) GetMessages(ctx *gin.Context) {
	roomID, _, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	messages, err := c.usecase.GetRoomMessages(ctx.Request.Context(), roomID, parseLimit(ctx, 50))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, MessagesResponse{
		Messages: c.toMessageResponses(messages),
		Count:    len(messages),
		RoomID:   roomID,
	})
}

func (c *messageController) GetMessagesAfter(ctx *gin.Context) {
	timestampStr := ctx.Query("timestamp")
	if timestampStr == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "timestamp parameter is required",
		})
		return
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "invalid timestamp format, use RFC3339 (e.g., 2024-01-01T12:00:00Z)",
		})
		return
	}

	roomID, _, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	messages, err := c.usecase.GetMessagesAfter(ctx.Request.Context(), roomID, timestamp, parseLimit(ctx, 100))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "fetch_failed",
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, MessagesResponse{
		Messages: c.toMessageResponses(messages),
		Count:    len(messages),
		RoomID:   roomID,
	})
}

func (c *messageController) GetMessageCount(ctx *gin.Context) {
	roomID, _, ok := c.requireRoomMember(ctx)
	if !ok {
		return
	}

	count, err := c.usecase.GetMessageCount(ctx.Request.Context(), roomID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "count_failed",
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, MessageCountResponse{
		RoomID: roomID,
		Count:  count,
	})
}

func (c *messageController) toMessageResponse(msg *model.Message) MessageResponse {
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

func (c *messageController) toMessageResponses(messages []*model.Message) []MessageResponse {
	responses := make([]MessageResponse, len(messages))
	for i, msg := range messages {
		responses[i] = c.toMessageResponse(msg)
	}
	return responses
}

// parseLimit reads the "limit" query parameter, falling back to def when it
// is missing or malformed. Range clamping is done by the use case.
func parseLimit(ctx *gin.Context, def int64) int64 {
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsed, err := strconv.ParseInt(limitStr, 10, 64); err == nil {
			return parsed
		}
	}
	return def
}

func isContentValidationError(err error) bool {
	msg := err.Error()
	return msg == "message cannot be empty" ||
		msg == "message cannot contain only whitespace" ||
		strings.HasPrefix(msg, "message cannot exceed")
}
