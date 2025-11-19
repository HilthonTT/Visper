package health

import (
	"net/http"
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/json"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	data := healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
	}
	json.Write(w, http.StatusOK, data)
}
