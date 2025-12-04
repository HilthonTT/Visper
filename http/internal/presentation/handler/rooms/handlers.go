package rooms

import (
	"errors"
	"log"
	"net/http"
	"strings"
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

	json.Write(w, http.StatusCreated, resp)
}

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

	log.Printf("User %s (%s) connected to room %s", existingMember.User.Name, memberToken, roomID)
}

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

	// Broadcast
	payload := ws.NewKicked(roomID, bootedMember.User.Name, "booted")
	h.core.Broadcast() <- payload
}

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

	w.WriteHeader(http.StatusNoContent)

	// Broadcast
	payload := ws.NewMemberLeft(roomID, memberThatLeft.User.ID, memberThatLeft.User.Name)
	h.core.Broadcast() <- payload
}

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
