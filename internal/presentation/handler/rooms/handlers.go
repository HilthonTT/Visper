package rooms

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

func (h *Handler) CreateRoomHandler(w http.ResponseWriter, r *http.Request) {
	var req createRoomRequest
	if err := json.Read(r, &req); err != nil {
		json.WriteValidationError(w, err)
		return
	}

	memberToken := utils.GetMemberToken(w, r)
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

	memberToken := utils.SetupMemberToken(w, r)
	if memberToken == "" {
		_ = conn.WriteJSON(ws.NewAuthError(roomID, "Authentication failed"))
		_ = conn.Close()
		return
	}

	user, err := domain.NewUser(username)
	if err != nil {
		_ = conn.WriteJSON(ws.NewError(roomID, "Invalid username"))
		_ = conn.Close()
		return
	}

	member := domain.NewMember(memberToken, user)

	room, err := h.roomRepository.GetByID(r.Context(), roomID)
	if err != nil {
		var msg string
		if errors.Is(err, domain.ErrRoomNotFound) {
			msg = "Room not found"
		} else {
			msg = "Failed to load room"
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

	if err := room.AddMember(member); err != nil {
		if errors.Is(err, domain.ErrAlreadyInRoom) {
			log.Printf("User %s rejoining room %s", member.User.ID, roomID)
		} else {
			_ = conn.WriteJSON(ws.NewJoinFailed(roomID, "Cannot join room"))
			_ = conn.Close()
			return
		}
	}

	if err := h.roomRepository.Update(r.Context(), room); err != nil {
		log.Printf("Failed to persist room %s after join: %v", roomID, err)
	}

	client := ws.NewClient(conn, member.User.ID, roomID, member.User.Name)

	h.roomManager.AddClient(client)
	h.core.Register() <- client

	go client.WriteMessage()
	go client.ReadMessage(h.core)

	log.Printf("User %s (%s) joined room %s", member.User.Name, member.User.ID, roomID)
}

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {

}
