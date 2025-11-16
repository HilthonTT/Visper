package rooms

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/hilthontt/visper/internal/domain"
	"github.com/hilthontt/visper/internal/infrastructure/json"
	"github.com/hilthontt/visper/internal/presentation/utils"
)

type Handler struct {
	roomRepository    domain.RoomRepository
	messageRepository domain.MessageRepository
}

func NewHandler(
	roomRepository domain.RoomRepository,
	messageRepository domain.MessageRepository,
) *Handler {
	return &Handler{
		roomRepository:    roomRepository,
		messageRepository: messageRepository,
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
