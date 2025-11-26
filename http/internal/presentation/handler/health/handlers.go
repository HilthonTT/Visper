package health

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/json"
)

var (
	startTime       = time.Now()
	healthy   int32 = 1 // 1: health, 0 = unhealthy
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&healthy) == 0 {
		json.Write(w, http.StatusServiceUnavailable, healthResponse{
			Status:    "unhealthy",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Uptime:    time.Since(startTime).Round(time.Second).String(),
		})
		return
	}

	json.Write(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(startTime).Round(time.Second).String(),
	})
}
