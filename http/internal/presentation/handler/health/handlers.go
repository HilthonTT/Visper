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

// GetHealth godoc
// @Summary      Health check
// @Description  Returns the health status of the API, including uptime and current timestamp
// @Tags         health
// @Produce      json
// @Success      200 {object} healthResponse "Service is healthy"
// @Failure      503 {object} healthResponse "Service is unhealthy"
// @Router       /health [get]
// @Router       /healthz [get]
// @Router       /ready [get]
// @Router       /live [get]
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
