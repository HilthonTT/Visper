package json

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, err error, msg string) {
	resp := ErrorResponse{
		Error:   http.StatusText(status),
		Message: msg,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func WriteValidationError(w http.ResponseWriter, err error) {
	WriteError(w, http.StatusBadRequest, err, err.Error())
}

func WriteInternalError(w http.ResponseWriter, err error) {
	log.Printf("Internal error: %v", err)
	WriteError(w, http.StatusInternalServerError, err, "An unexpected error occurred")
}

func WriteRateLimitError(w http.ResponseWriter, retryAfter int) {
	resp := ErrorResponse{
		Error:   http.StatusText(http.StatusTooManyRequests),
		Message: "Too many requests. Please try again later.",
	}

	w.Header().Set("Content-Type", "application/json")
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	}
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(resp)
}
