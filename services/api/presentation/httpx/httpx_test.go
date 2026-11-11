package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/application/usecases/file"
	"github.com/hilthontt/visper/services/api/application/usecases/message"
	"github.com/hilthontt/visper/services/api/application/usecases/room"
	"github.com/hilthontt/visper/services/api/domain/apperr"
)

func TestErrorMapsKindsToStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		err     error
		status  int
		message string
	}{
		{room.ErrRoomNotFound, http.StatusNotFound, "room not found"},
		{room.ErrRoomExpired, http.StatusNotFound, "room has expired"},
		{room.ErrOwnerOnlyKick, http.StatusForbidden, "only the room owner can kick members"},
		{room.ErrInvalidCodes, http.StatusForbidden, "invalid join code or secure token"},
		{file.ErrRoomExpired, http.StatusGone, "room has expired"},
		{message.ErrEmptyMessage, http.StatusBadRequest, "message cannot be empty"},
		{apperr.Wrap(apperr.TooLarge, errors.New("too big")), http.StatusRequestEntityTooLarge, "too big"},
		// Wrapping keeps the kind.
		{fmt.Errorf("context: %w", room.ErrRoomNotFound), http.StatusNotFound, "context: room not found"},
		// Internal details are not sent to the client.
		{errors.New("redis: connection refused"), http.StatusInternalServerError, internalErrorMessage},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		Error(ctx, tt.err)

		var body ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != tt.status || body.Message != tt.message {
			t.Errorf("%v: got %d %q, want %d %q", tt.err, w.Code, body.Message, tt.status, tt.message)
		}
	}
}
