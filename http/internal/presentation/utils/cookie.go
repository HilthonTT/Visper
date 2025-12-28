package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/internal/domain"
)

const (
	CookieNameAnon     = "roomanonauth"
	CookieRoomAuth     = "roomauth"
	CookieNameJS       = "roomauth_js"
	CookieNameMemberID = "member_id"
)

func setMemberCookie(w http.ResponseWriter, name string, member *domain.Member, path string, httpOnly bool, expires time.Time) {
	data, _ := json.Marshal(member)
	value := base64.StdEncoding.EncodeToString(data)

	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: httpOnly,
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

func SetAuthenticatedMemberCookies(member *domain.Member, roomPath string, w http.ResponseWriter) {
	expires := time.Now().Add(240 * time.Hour)
	setMemberCookie(w, CookieRoomAuth, member, roomPath, true, expires)
	setMemberCookie(w, CookieNameJS, member, roomPath, false, expires)
}

func ClearAuthenticatedMemberCookies(roomPath string, w http.ResponseWriter) {
	past := time.Now().Add(-24 * time.Hour)

	setMemberCookie(w, CookieRoomAuth, &domain.Member{}, roomPath, true, past)
	setMemberCookie(w, CookieNameJS, &domain.Member{}, roomPath, false, past)
}

func SetAnonymousMemberCookie(member *domain.Member, roomPath string, w http.ResponseWriter) {
	expires := time.Now().Add(240 * time.Hour)
	setMemberCookie(w, CookieNameAnon, member, roomPath, true, expires)
}

func FormatRoomPath(roomID string) string {
	return fmt.Sprintf("/rooms/%s", url.QueryEscape(roomID))
}

func EnsureMemberID(w http.ResponseWriter, r *http.Request) string {
	if id := GetMemberIDFromCookie(r); id != "" {
		return id
	}
	newID := uuid.New().String()
	SetPersistentMemberIDCookie(newID, w)
	return newID
}

func GetMemberIDFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(CookieNameMemberID)
	if err != nil {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}
	return string(decoded)
}

func SetPersistentMemberIDCookie(memberID string, w http.ResponseWriter) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     CookieNameMemberID,
		Value:    base64.StdEncoding.EncodeToString([]byte(memberID)),
		Path:     "/",
		HttpOnly: true,
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
}

func GetMemberIDFromRequest(r *http.Request) string {
	// First try header (for API clients)
	if token := r.Header.Get("X-Member-Token"); token != "" {
		return token
	}

	// Fall back to cookie (for WebSocket)
	return GetMemberIDFromCookie(r)
}
