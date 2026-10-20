package auth

import (
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

const sessionCookieName = "notebook_session"

type sessionData struct {
	UserID int64
	Expiry int64
}

type SessionCodec struct {
	sc *securecookie.SecureCookie
}

func NewSessionCodec(hashKey, blockKey []byte) *SessionCodec {
	return &SessionCodec{sc: securecookie.New(hashKey, blockKey)}
}

func (c *SessionCodec) SetSession(w http.ResponseWriter, userID int64, ttl time.Duration) error {
	data := sessionData{UserID: userID, Expiry: time.Now().Add(ttl).Unix()}
	encoded, err := c.sc.Encode(sessionCookieName, data)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
	return nil
}

func (c *SessionCodec) ReadSession(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, false
	}
	var data sessionData
	if err := c.sc.Decode(sessionCookieName, cookie.Value, &data); err != nil {
		return 0, false
	}
	if time.Now().Unix() > data.Expiry {
		return 0, false
	}
	return data.UserID, true
}

func (c *SessionCodec) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
