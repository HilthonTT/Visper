package message

import (
	"net/http"
	"strconv"
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
	usecase       message.MessageUseCase
	roomUseCase   room.RoomUseCase
	wsRoomManager *websocket.RoomManager
	wsCore        *websocket.Core
}

func NewMessageController(
	usecase message.MessageUseCase,
	roomUseCase room.RoomUseCase,
	wsRoomManager *websocket.RoomManager,
	wsCore *websocket.Core,
) MessageController {
	return &messageController{
		usecase:       usecase,
		roomUseCase:   roomUseCase,
		wsRoomManager: wsRoomManager,
		wsCore:        wsCore,
	}
}

func (c *messageController) DeleteMessage(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	messageID := ctx.Param("messageId")
	if messageID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "message ID is required",
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

	room, err := c.roomUseCase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "room not found",
		})
		return
	}

	if !room.IsMember(user.ID) {
		ctx.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "you are not a member of this room",
		})
		return
	}

	err = c.usecase.Delete(ctx.Request.Context(), roomID, messageID, user.ID)
	if err != nil {
		status := http.StatusInternalServerError
		errorCode := "delete_failed"

		switch {
		case err.Error() == "message not found":
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

	now := time.Now()
	wsMessage := websocket.NewMessageDeleted(roomID, messageID, now.String())
	c.wsCore.Broadcast() <- wsMessage

	ctx.JSON(http.StatusOK, MessageDeletedResponse{
		Success:   true,
		MessageID: messageID,
	})
}

func (c *messageController) UpdateMessage(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

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

	user, exists := middlewares.GetUserFromContext(ctx)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "user not found in context",
		})
		return
	}

	room, err := c.roomUseCase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "room not found",
		})
		return
	}

	if !room.IsMember(user.ID) {
		ctx.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "you are not a member of this room",
		})
		return
	}

	err = c.usecase.Update(ctx.Request.Context(), roomID, messageID, user.ID, req.Content)
	if err != nil {
		status := http.StatusInternalServerError
		errorCode := "update_failed"

		switch {
		case err.Error() == "message not found":
			status = http.StatusNotFound
			errorCode = "not_found"
		case err.Error() == "unauthorized: you can only edit your own messages":
			status = http.StatusForbidden
			errorCode = "forbidden"
		case err.Error() == "message cannot be empty" ||
			err.Error() == "message cannot contain only whitespace":
			status = http.StatusBadRequest
			errorCode = "invalid_content"
		}

		ctx.JSON(status, ErrorResponse{
			Error:   errorCode,
			Message: err.Error(),
		})
		return
	}

	now := time.Now()
	wsMessage := websocket.NewMessageUpdated(roomID, messageID, req.Content, now.String())
	c.wsCore.Broadcast() <- wsMessage

	ctx.JSON(http.StatusOK, MessageUpdatedResponse{
		Success:   true,
		MessageID: messageID,
		Content:   req.Content,
	})
}

func (c *messageController) SendMessage(ctx *gin.Context) {
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

	var req SendMessageRequest
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

	msg, err := c.usecase.Send(ctx.Request.Context(), roomID, user.ID, user.Username, req.Content)
	if err != nil {
		status := http.StatusInternalServerError
		// Check for validation errors
		if err.Error() == "message cannot be empty" ||
			err.Error() == "message cannot contain only whitespace" {
			status = http.StatusBadRequest
		}
		ctx.JSON(status, ErrorResponse{
			Error:   "send_failed",
			Message: err.Error(),
		})
		return
	}

	wsMessage := websocket.NewMessageReceived(roomID, msg.ID, msg.Content, msg.UserID, msg.Username, msg.CreatedAt.String())
	c.wsCore.Broadcast() <- wsMessage

	ctx.JSON(http.StatusCreated, c.toMessageResponse(msg))
}

func (c *messageController) GetMessages(ctx *gin.Context) {
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

	room, err := c.roomUseCase.GetByID(ctx.Request.Context(), roomID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not-found",
			Message: "room not found",
		})
		return
	}

	if !room.IsMember(user.ID) {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "you are not a member of this room",
		})
		return
	}

	limit := int64(50) // default
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.ParseInt(limitStr, 10, 64); err == nil {
			limit = parsedLimit
		}
	}

	messages, err := c.usecase.GetRoomMessages(ctx.Request.Context(), roomID, limit)
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
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
		return
	}

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

	limit := int64(100)
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.ParseInt(limitStr, 10, 64); err == nil {
			limit = parsedLimit
		}
	}

	messages, err := c.usecase.GetMessagesAfter(ctx.Request.Context(), roomID, timestamp, limit)
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
	roomID := ctx.Param("id")
	if roomID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "room ID is required",
		})
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
	}
}

func (c *messageController) toMessageResponses(messages []*model.Message) []MessageResponse {
	responses := make([]MessageResponse, len(messages))
	for i, msg := range messages {
		responses[i] = c.toMessageResponse(msg)
	}
	return responses
}
