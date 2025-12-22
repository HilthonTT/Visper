package rooms

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hilthontt/visper/internal/domain"
	"github.com/hilthontt/visper/internal/infrastructure/events"
	"github.com/hilthontt/visper/internal/infrastructure/json"
	"github.com/hilthontt/visper/internal/infrastructure/ws"
	"github.com/hilthontt/visper/internal/presentation/utils"
)

type Handler struct {
	roomRepository    domain.RoomRepository
	messageRepository domain.MessageRepository
	roomManager       *ws.RoomManager
	core              *ws.Core
	roomPublisher     *events.RoomPublisher
}

func NewHandler(
	roomRepository domain.RoomRepository,
	messageRepository domain.MessageRepository,
	roomManager *ws.RoomManager,
	core *ws.Core,
	roomPublisher *events.RoomPublisher,
) *Handler {
	return &Handler{
		roomRepository:    roomRepository,
		messageRepository: messageRepository,
		roomManager:       roomManager,
		core:              core,
		roomPublisher:     roomPublisher,
	}
}

// CreateRoomHandler godoc
// @Summary      Create a new chat room
// @Description  Creates a new chat room with the specified settings and returns room details
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        request body createRoomRequest true "Room creation parameters"
// @Success      201 {object} createRoomResponse "Room created successfully"
// @Failure      400 {object} map[string]interface{} "Bad request - validation error or room full"
// @Failure      401 {object} map[string]interface{} "Unauthorized - missing authentication"
// @Failure      409 {object} map[string]interface{} "Conflict - room already exists or user already in room"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     MemberAuth
// @Router       /rooms [post]
func (h *Handler) CreateRoomHandler(w http.ResponseWriter, r *http.Request) {
	var req createRoomRequest
	if err := json.Read(r, &req); err != nil {
		json.WriteValidationError(w, err)
		return
	}

	memberToken := utils.EnsureMemberID(w, r)
	if memberToken == "" {
		json.WriteError(w, http.StatusUnauthorized, errors.New("unauthorized"), "Missing or invalid authentication")
		return
	}

	user, err := domain.NewUser(req.Username)
	if err != nil {
		json.WriteValidationError(w, err)
		return
	}

	member := domain.NewMember(memberToken, user)

	newRoom, err := domain.NewRoom(member, req.Persistent, time.Hour)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrRoomFull):
			json.WriteError(w, http.StatusBadRequest, err, "Cannot create room: too many members")
		case errors.Is(err, domain.ErrAlreadyInRoom):
			json.WriteError(w, http.StatusConflict, err, "You are already in a room")
		default:
			log.Printf("Failed to create room for member %s: %v", memberToken, err)
			json.WriteInternalError(w, err)
		}
		return
	}

	ctx := r.Context()
	if err := h.roomRepository.Create(ctx, newRoom); err != nil {
		switch {
		case errors.Is(err, domain.ErrRoomAlreadyExists):
			json.WriteError(w, http.StatusConflict, err, "Room already exists")
		default:
			log.Printf("Repository error creating room %s: %v", newRoom.ID, err)
			json.WriteInternalError(w, err)
		}
		return
	}

	resp := createRoomResponse{
		RoomID:     newRoom.ID,
		JoinCode:   newRoom.JoinCode,
		CreatedAt:  newRoom.CreatedAt,
		Persistent: newRoom.Persistent,
	}

	if err := h.roomPublisher.PublishRoomCreated(ctx, *newRoom); err != nil {
		log.Printf("Error publishing room created: %v\n", err)
	}

	json.Write(w, http.StatusCreated, resp)
}

// JoinRoomHandler godoc
// @Summary      Join a chat room via WebSocket
// @Description  Establishes WebSocket connection to join a chat room with the provided join code
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        roomId path string true "Room ID"
// @Param        joinCode query string true "Join code for the room"
// @Param        username query string true "Username to join with"
// @Success      101 {object} map[string]interface{} "Switching Protocols - WebSocket connection established"
// @Failure      400 {object} map[string]interface{} "Bad request - missing parameters"
// @Failure      401 {object} map[string]interface{} "Unauthorized - authentication failed"
// @Failure      404 {object} map[string]interface{} "Room not found"
// @Security     MemberAuth
// @Router       /rooms/{roomId}/join [get]
func (h *Handler) JoinRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("room ID is missing"))
		return
	}

	joinCode := r.URL.Query().Get("joinCode")
	if joinCode == "" {
		json.WriteValidationError(w, errors.New("joinCode query parameter is required"))
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		json.WriteValidationError(w, errors.New("username query parameter is required"))
		return
	}

	conn, err := h.roomManager.Upgrade(w, r)
	if err != nil {
		log.Printf("WebSocket upgrade failed for room %s: %v", roomID, err)
		return
	}

	memberToken := utils.EnsureMemberID(w, r)
	if memberToken == "" {
		_ = conn.WriteJSON(ws.NewAuthError(roomID, "Failed to establish identity"))
		_ = conn.Close()
		return
	}

	room, err := h.roomRepository.GetByID(r.Context(), roomID)
	if err != nil {
		msg := "Failed to load room"
		if errors.Is(err, domain.ErrRoomNotFound) {
			msg = "Room not found"
		}
		_ = conn.WriteJSON(ws.NewJoinFailed(roomID, msg))
		_ = conn.Close()
		return
	}

	if room.JoinCode != joinCode {
		_ = conn.WriteJSON(ws.NewJoinFailed(roomID, "Invalid join code"))
		_ = conn.Close()
		return
	}

	wasAlreadyMember := false
	existingMember := room.FindMemberByID(memberToken)

	if existingMember != nil {
		log.Printf("User rejoining room %s: %s (%s)", roomID, existingMember.User.Name, memberToken)
		wasAlreadyMember = true
	} else {
		user, err := domain.NewUser(username)
		if err != nil {
			_ = conn.WriteJSON(ws.NewError(roomID, "Invalid username"))
			_ = conn.Close()
			return
		}

		existingMember = domain.NewMember(memberToken, user)

		if err := room.AddMember(existingMember); err != nil {
			_ = conn.WriteJSON(ws.NewJoinFailed(roomID, "Cannot join room"))
			_ = conn.Close()
			return
		}

		if err := h.roomRepository.Update(r.Context(), room); err != nil {
			log.Printf("Failed to persist room %s after new member join: %v", roomID, err)
		}
	}

	roomPath := utils.FormatRoomPath(room.ID)
	utils.SetAuthenticatedMemberCookies(existingMember, roomPath, w)

	client := ws.NewClient(conn, existingMember.User.ID, roomID, existingMember.User.Name)

	h.roomManager.AddClient(client)
	h.core.Register() <- client

	go client.WriteMessage()
	go client.ReadMessage(h.core)

	// Broadcast join
	if !wasAlreadyMember {
		wsPayload := ws.NewMemberJoined(roomID, ws.MemberPayload{
			UserID:   existingMember.User.ID,
			Username: existingMember.User.Name,
			JoinedAt: time.Now().UTC().Format(time.RFC3339),
		})
		h.core.Broadcast() <- wsPayload
	}

	if err := h.roomPublisher.PublishRoomJoined(r.Context(), *room); err != nil {
		log.Printf("Error publishing room joined: %v\n", err)
	}

	log.Printf("User %s (%s) connected to room %s", existingMember.User.Name, memberToken, roomID)
}

// BootUserHandler godoc
// @Summary      Boot a user from room
// @Description  Removes a user from the room (owner only)
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        roomId path string true "Room ID"
// @Param        request body bootUserRequest true "User to boot"
// @Success      204 "User booted successfully"
// @Failure      400 {object} map[string]interface{} "Bad request - cannot boot yourself"
// @Failure      401 {object} map[string]interface{} "Unauthorized - not owner or not member"
// @Failure      404 {object} map[string]interface{} "Room or member not found"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     MemberAuth
// @Router       /rooms/{roomId}/boot [post]
func (h *Handler) BootUserHandler(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("room ID is missing"))
		return
	}

	var req bootUserRequest
	if err := json.Read(r, &req); err != nil {
		json.WriteValidationError(w, err)
		return
	}

	currentMemberID := utils.GetMemberIDFromCookie(r)
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

	if !room.IsOwner(existingMember) {
		json.WriteError(w, http.StatusUnauthorized, errors.New("unauthorized"), "You aren't the owner")
		return
	}

	if strings.EqualFold(currentMemberID, req.MemberID) {
		json.WriteBadRequestError(w, "You cannot boot yourself")
		return
	}

	bootedMember, err := h.roomRepository.RemoveMember(r.Context(), roomID, req.MemberID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrMemberNotFound):
			json.WriteError(w, http.StatusNotFound, err, "Member not found")
		default:
			log.Printf("Failed to boot user: %v", err)
			json.WriteInternalError(w, err)
		}
	}

	w.WriteHeader(http.StatusNoContent)

	if err := h.roomPublisher.PublishRoomMemberKicked(r.Context(), *room); err != nil {
		log.Printf("Error publishing room kicked: %v\n", err)
	}

	// Broadcast
	payload := ws.NewKicked(roomID, bootedMember.User.Name, "booted")
	h.core.Broadcast() <- payload
}

// LeaveRoomHandler godoc
// @Summary      Leave a chat room
// @Description  Removes the current user from the room
// @Tags         rooms
// @Produce      json
// @Param        roomId path string true "Room ID"
// @Success      204 "Left room successfully"
// @Failure      400 {object} map[string]interface{} "Bad request - missing room ID"
// @Failure      401 {object} map[string]interface{} "Unauthorized - not a member"
// @Failure      404 {object} map[string]interface{} "Room or member not found"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     MemberAuth
// @Router       /rooms/{roomId}/leave [post]
func (h *Handler) LeaveRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("room ID is missing"))
		return
	}

	currentMemberID := utils.GetMemberIDFromCookie(r)
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

	memberThatLeft, err := h.roomRepository.RemoveMember(r.Context(), roomID, existingMember.Token)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrMemberNotFound):
			json.WriteError(w, http.StatusNotFound, err, "Member not found")
		default:
			log.Printf("Failed to leave room: %v", err)
			json.WriteInternalError(w, err)
		}
	}

	if err := h.roomPublisher.PublishRoomLeave(r.Context(), *room); err != nil {
		log.Printf("Error publishing room leave: %v\n", err)
	}

	w.WriteHeader(http.StatusNoContent)

	// Broadcast
	payload := ws.NewMemberLeft(roomID, memberThatLeft.User.ID, memberThatLeft.User.Name)
	h.core.Broadcast() <- payload
}

// DeleteRoomHandler godoc
// @Summary      Delete a chat room
// @Description  Permanently deletes a room and all its messages (owner only)
// @Tags         rooms
// @Produce      json
// @Param        roomId path string true "Room ID"
// @Success      204 "Room deleted successfully"
// @Failure      400 {object} map[string]interface{} "Bad request - missing room ID"
// @Failure      401 {object} map[string]interface{} "Unauthorized - not owner or not member"
// @Failure      404 {object} map[string]interface{} "Room not found"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     MemberAuth
// @Router       /rooms/{roomId} [delete]
func (h *Handler) DeleteRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("room ID is missing"))
		return
	}

	currentMemberID := utils.GetMemberIDFromCookie(r)
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

	if room.Owner.User.ID != existingMember.User.ID {
		json.WriteError(w, http.StatusUnauthorized, errors.New("unauthorized"), "You are not authorized to perform this action")
		return
	}

	deletedRoom, err := h.roomRepository.Delete(r.Context(), room)
	if err != nil {
		json.WriteInternalError(w, err)
		return
	}

	if err := h.messageRepository.DeleteByRoomID(r.Context(), roomID); err != nil {
		json.WriteInternalError(w, err)
		return
	}

	if err := h.roomPublisher.PublishRoomDeleted(r.Context(), *deletedRoom); err != nil {
		log.Printf("Error publishing room deleted: %v\n", err)
	}

	w.WriteHeader(http.StatusNoContent)

	// Broadcast
	payload := ws.NewRoomDeleted(deletedRoom.ID)
	h.core.Broadcast() <- payload
}

// GetRoomHandler godoc
// @Summary      Get room details
// @Description  Retrieves room information including messages and members
// @Tags         rooms
// @Produce      json
// @Param        roomId path string true "Room ID"
// @Success      200 {object} roomResponse "Room details"
// @Failure      400 {object} map[string]interface{} "Bad request - missing room ID"
// @Failure      401 {object} map[string]interface{} "Unauthorized - not a member"
// @Failure      404 {object} map[string]interface{} "Room not found"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     MemberAuth
// @Router       /rooms/{roomId} [get]
func (h *Handler) GetRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomId")
	if roomID == "" {
		json.WriteValidationError(w, errors.New("room ID is missing"))
		return
	}

	currentMemberID := utils.GetMemberIDFromCookie(r)
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

	messages, err := h.messageRepository.GetByRoomID(r.Context(), roomID)
	if err != nil {
		json.WriteInternalError(w, err)
		return
	}

	mappedMessages := make([]messageResponse, 0, len(messages))

	for i, message := range messages {
		mappedMessages[i] = messageResponse{
			ID:      message.ID,
			Content: message.Content,
			User: userResponse{
				ID:   message.User.ID,
				Name: message.User.Name,
			},
		}
	}

	resp := roomResponse{
		ID:       room.ID,
		JoinCode: room.JoinCode,
		Owner: userResponse{
			ID:   room.Owner.User.ID,
			Name: room.Owner.User.Name,
		},
		Messages:   mappedMessages,
		Persistent: room.Persistent,
		CreatedAt:  room.CreatedAt,
	}

	json.Write(w, http.StatusOK, resp)
}
