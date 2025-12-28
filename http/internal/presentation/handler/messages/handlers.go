package messages

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hilthontt/visper/internal/domain"
	"github.com/hilthontt/visper/internal/infrastructure/json"
	"github.com/hilthontt/visper/internal/infrastructure/ws"
	"github.com/hilthontt/visper/internal/presentation/utils"
)

type Handler struct {
	roomRepository    domain.RoomRepository
	messageRepository domain.MessageRepository
	roomManager       *ws.RoomManager
	core              *ws.Core
}

func NewHandler(
	roomRepository domain.RoomRepository,
	messageRepository domain.MessageRepository,
	roomManager *ws.RoomManager,
	core *ws.Core,
) *Handler {
	return &Handler{
		roomRepository:    roomRepository,
		messageRepository: messageRepository,
		roomManager:       roomManager,
		core:              core,
	}
}

// CreateNewMessageHandler godoc
// @Summary      Create a new message
// @Description  Creates a new message in the specified room and broadcasts it to all connected clients. Messages are persisted only if the room is persistent.
// @Tags         messages
// @Accept       json
// @Produce      json
// @Param        roomId path string true "Room ID"
// @Param        request body createMessageRequest true "Message content"
// @Success      201 {object} createMessageResponse "Message created successfully"
// @Failure      400 {object} map[string]interface{} "Bad request - validation error or missing room ID"
// @Failure      401 {object} map[string]interface{} "Unauthorized - missing authentication or not a member"
// @Failure      404 {object} map[string]interface{} "Room not found"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     MemberAuth
// @Router       /rooms/{roomId}/messages [post]
func (h *Handler) CreateNewMessageHandler(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("room ID is missing"))
		return
	}

	var req createMessageRequest
	if err := json.Read(r, &req); err != nil {
		json.WriteValidationError(w, err)
		return
	}

	currentMemberID := utils.GetMemberIDFromRequest(r)
	if currentMemberID == "" {
		json.WriteError(w, http.StatusUnauthorized, errors.New("unauthorized"), "Missing or invalid authentication")
		return
	}

	room, err := h.roomRepository.GetByID(r.Context(), roomID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrRoomNotFound):
			json.WriteError(w, http.StatusNotFound, err, "Room not found")
		default:
			log.Printf("Failed to find room: %v", err)
			json.WriteInternalError(w, err)
		}
		return
	}

	existingMember := room.FindMemberByID(currentMemberID)

	// If not found by token, check if it's the owner's token
	if existingMember == nil && room.Owner != nil && room.Owner.Token == currentMemberID {
		existingMember = room.Owner
	}

	if existingMember == nil {
		log.Printf("Member not found - currentMemberID: %s, room members: %+v", currentMemberID, room.Members)
		json.WriteError(w, http.StatusUnauthorized, errors.New("unauthorized"), "You are not a member")
		return
	}

	message, err := domain.NewMessage(existingMember, req.Content, room.ID)
	if err != nil {
		json.WriteValidationError(w, err)
		return
	}

	if room.Persistent {
		if err := h.messageRepository.Create(r.Context(), message); err != nil {
			log.Printf("Failed to persist message: %v\n", err)
		}
	}

	resp := createMessageResponse{
		ID:      message.ID,
		RoomID:  message.RoomID,
		Content: message.Content,
	}

	wsPayload := ws.NewMessageReceived(
		roomID,
		message.ID,
		message.Content,
		message.User.ID,
		message.User.Name,
		message.CreatedAt.Format(time.RFC3339),
	)

	json.Write(w, http.StatusCreated, resp)

	// Broadcast to room
	h.core.Broadcast() <- wsPayload
}

// DeleteMessageHandler godoc
// @Summary      Delete a message
// @Description  Deletes a message from the room. Only available for persistent rooms.
// @Tags         messages
// @Produce      json
// @Param        roomId path string true "Room ID"
// @Param        messageId path string true "Message ID"
// @Success      204 "Message deleted successfully"
// @Failure      400 {object} map[string]interface{} "Bad request - missing room ID or message ID"
// @Failure      401 {object} map[string]interface{} "Unauthorized - missing authentication or not a member"
// @Failure      404 {object} map[string]interface{} "Room or message not found"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     MemberAuth
// @Router       /rooms/{roomId}/messages/{messageId} [delete]
func (h *Handler) DeleteMessageHandler(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("room ID is missing"))
		return
	}

	messageID := chi.URLParam(r, "messageId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("message ID is missing"))
		return
	}

	currentMemberID := utils.GetMemberIDFromRequest(r)
	if currentMemberID == "" {
		json.WriteError(w, http.StatusUnauthorized, errors.New("unauthorized"), "Missing or invalid authentication")
		return
	}

	room, err := h.roomRepository.GetByID(r.Context(), roomID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrRoomNotFound):
			json.WriteError(w, http.StatusNotFound, err, "Room not found")
		default:
			log.Printf("Failed to find room: %v", err)
			json.WriteInternalError(w, err)
		}
		return
	}

	existingMember := room.FindMemberByID(currentMemberID)
	if existingMember == nil {
		json.WriteError(w, http.StatusUnauthorized, errors.New("unauthorized"), "You are not a member")
		return
	}

	messageToDelete, err := h.messageRepository.GetByID(r.Context(), room.ID, messageID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrMessageNotFound):
			json.WriteError(w, http.StatusNotFound, err, "Message not found")
		default:
			log.Printf("Failed to find message: %v", err)
			json.WriteInternalError(w, err)
		}
		return
	}

	if err := h.messageRepository.Delete(r.Context(), messageToDelete); err != nil {
		json.WriteInternalError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)

	// Broadcast
	wsPayload := ws.NewMessageDeleted(roomID, messageToDelete.ID)
	h.core.Broadcast() <- wsPayload
}
