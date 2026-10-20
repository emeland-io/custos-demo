package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

const flowCookieName = "notebook_oidc_flow"

var flowTTL = 10 * time.Minute

type flowData struct {
	State    string
	Verifier string
}

type FlowCodec struct {
	sc *securecookie.SecureCookie
}

func NewFlowCodec(hashKey, blockKey []byte) *FlowCodec {
	return &FlowCodec{sc: securecookie.New(hashKey, blockKey).MaxAge(int(flowTTL.Seconds()))}
}

func (c *FlowCodec) SetFlow(w http.ResponseWriter, state, verifier string) error {
	encoded, err := c.sc.Encode(flowCookieName, flowData{State: state, Verifier: verifier})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookieName,
		Value:    encoded,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(flowTTL.Seconds()),
	})
	return nil
}

func (c *FlowCodec) ReadFlow(r *http.Request) (string, string, bool) {
	cookie, err := r.Cookie(flowCookieName)
	if err != nil {
		return "", "", false
	}
	var data flowData
	if err := c.sc.Decode(flowCookieName, cookie.Value, &data); err != nil {
		return "", "", false
	}
	return data.State, data.Verifier, true
}

func (c *FlowCodec) ClearFlow(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func randomURLSafeString(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewPKCEVerifier returns a random code verifier per RFC 7636 (43-128
// chars once base64url-encoded; 32 random bytes encodes to 43 chars).
func NewPKCEVerifier() (string, error) {
	return randomURLSafeString(32)
}

// PKCEChallenge derives the S256 code challenge from a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func NewState() (string, error) {
	return randomURLSafeString(16)
}
