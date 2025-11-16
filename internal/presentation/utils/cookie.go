package utils

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/internal/domain"
)

const (
	CookieMemberID = "member_id"
	CookieRoomAuth = "room_auth"
)

func GetMemberToken(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(CookieMemberID)
	if err != nil {
		return ""
	}

	decoded, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}

	decodedStr := string(decoded)
	if decodedStr == "" {
		decodedStr = uuid.NewString()
	}

	setMemberIDCookie(decodedStr, w)
	return decodedStr
}

func GetMemberFromCookie(r *http.Request) (*domain.Member, error) {
	member, err := getCookieMemberDetails(CookieRoomAuth, r)
	if err != nil {
		return nil, err
	}

	return member, nil
}

func setMemberIDCookie(memberToken string, w http.ResponseWriter) {
	cookieExpiry := time.Now().Add(24 * 30 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     CookieMemberID,
		Value:    base64.StdEncoding.EncodeToString([]byte(memberToken)),
		Path:     "/",
		HttpOnly: true,
		Expires:  cookieExpiry,
	})
}

func getCookieMemberDetails(cookieName string, r *http.Request) (*domain.Member, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, errors.New("you're not a member of the room")
	}
	decoded, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil, errors.New("could not verify your membership in the room")
	}

	member := &domain.Member{}
	err = json.Unmarshal(decoded, member)
	if err != nil {
		return nil, errors.New("could not verify your membership in the room")
	}
	return member, nil
}
