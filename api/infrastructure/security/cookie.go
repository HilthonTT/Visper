package security

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/api/domain/model"
)

const (
	userIDCookie     = "visper_user_id"
	roomAuthCookie   = "visper_room_auth"
	roomAuthJSCookie = "visper_room_auth_js"

	userIDLifetime   = 30 * 24 * time.Hour // 30 days
	roomAuthLifetime = 10 * 24 * time.Hour // 10 days
)

type cookieConfig struct {
	name     string
	value    string
	path     string
	httpOnly bool
	maxAge   int
}

func setSecureCookie(w http.ResponseWriter, cfg cookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.name,
		Value:    cfg.value,
		Path:     cfg.path,
		HttpOnly: cfg.httpOnly,
		MaxAge:   cfg.maxAge,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

func encodeToBase64(data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(jsonData), nil
}

func decodeFromBase64(encoded string, target any) error {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}

	return json.Unmarshal(decoded, target)
}

func GetOrCreateUserID(w http.ResponseWriter, r *http.Request) string {
	if userID := GetUserID(r); userID != "" {
		return userID
	}

	newUserID := uuid.NewString()
	SetUserID(w, newUserID)
	return newUserID
}

func GetUserID(r *http.Request) string {
	// Check header first (for API/WebSocket clients)
	if headerUserID := r.Header.Get("X-User-ID"); headerUserID != "" {
		return headerUserID
	}

	// Fall back to cookie
	cookie, err := r.Cookie(userIDCookie)
	if err != nil {
		return ""
	}

	decoded, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}

	return string(decoded)
}

func SetUserID(w http.ResponseWriter, userID string) {
	encoded := base64.StdEncoding.EncodeToString([]byte(userID))

	setSecureCookie(w, cookieConfig{
		name:     userIDCookie,
		value:    encoded,
		path:     "/",
		httpOnly: true,
		maxAge:   int(userIDLifetime.Seconds()),
	})
}

// Room Authentication

// SetRoomAuth sets authentication cookies for a room
func SetRoomAuth(w http.ResponseWriter, user *model.User, roomID string) error {
	encoded, err := encodeToBase64(user)
	if err != nil {
		return err
	}

	roomPath := getRoomPath(roomID)
	maxAge := int(roomAuthLifetime.Seconds())

	// HttpOnly cookie for server-side auth
	setSecureCookie(w, cookieConfig{
		name:     roomAuthCookie,
		value:    encoded,
		path:     roomPath,
		httpOnly: true,
		maxAge:   maxAge,
	})

	// JavaScript-accessible cookie for client-side state
	setSecureCookie(w, cookieConfig{
		name:     roomAuthJSCookie,
		value:    encoded,
		path:     roomPath,
		httpOnly: false,
		maxAge:   maxAge,
	})

	return nil
}

func ClearRoomAuth(w http.ResponseWriter, roomID string) {
	roomPath := getRoomPath(roomID)

	setSecureCookie(w, cookieConfig{
		name:     roomAuthCookie,
		value:    "",
		path:     roomPath,
		httpOnly: true,
		maxAge:   -1,
	})

	setSecureCookie(w, cookieConfig{
		name:     roomAuthJSCookie,
		value:    "",
		path:     roomPath,
		httpOnly: false,
		maxAge:   -1,
	})
}

func GetRoomAuth(r *http.Request) (*model.User, error) {
	cookie, err := r.Cookie(roomAuthCookie)
	if err != nil {
		return nil, err
	}

	var user model.User
	if err := decodeFromBase64(cookie.Value, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func getRoomPath(roomID string) string {
	return "/rooms/" + roomID
}
