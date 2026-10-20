package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeOIDCServer struct {
	Server       *httptest.Server
	IssuerURL    string
	ClientID     string
	ClientSecret string

	key         *rsa.PrivateKey
	issuedCodes map[string]*issuedCode
}

type issuedCode struct {
	claims        map[string]any
	codeChallenge string
}

func newFakeOIDCServer(t *testing.T) *fakeOIDCServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	f := &fakeOIDCServer{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		key:          key,
		issuedCodes:  map[string]*issuedCode{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.handleDiscovery)
	mux.HandleFunc("/token", f.handleToken)
	mux.HandleFunc("/keys", f.handleKeys)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	f.IssuerURL = f.Server.URL

	return f
}

func (f *fakeOIDCServer) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                f.IssuerURL,
		"authorization_endpoint":                f.IssuerURL + "/authorize",
		"token_endpoint":                        f.IssuerURL + "/token",
		"jwks_uri":                              f.IssuerURL + "/keys",
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"userinfo_endpoint":                     f.IssuerURL + "/userinfo",
	})
}

func (f *fakeOIDCServer) handleKeys(w http.ResponseWriter, r *http.Request) {
	jwk := map[string]any{
		"kty": "RSA",
		"use": "sig",
		"kid": "test-key",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(f.key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.key.PublicKey.E)).Bytes()),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{jwk}})
}

// IssueCode registers claims and the PKCE code_challenge for a fresh,
// single-use authorization code. /token will exchange it for a signed
// ID token containing exactly those claims (plus iss/aud/exp/iat), but
// only if the matching code_verifier is presented.
func (f *fakeOIDCServer) IssueCode(claims map[string]any, codeChallenge string) string {
	code := fmt.Sprintf("code-%d", len(f.issuedCodes)+1)
	f.issuedCodes[code] = &issuedCode{claims: claims, codeChallenge: codeChallenge}
	return code
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (f *fakeOIDCServer) handleToken(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	issued, ok := f.issuedCodes[code]
	if !ok {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
		return
	}
	delete(f.issuedCodes, code)

	verifier := r.FormValue("code_verifier")
	if issued.codeChallenge == "" || pkceChallenge(verifier) != issued.codeChallenge {
		http.Error(w, "invalid_grant: PKCE verification failed", http.StatusBadRequest)
		return
	}

	now := time.Now()
	allClaims := jwt.MapClaims{
		"iss": f.IssuerURL,
		"aud": f.ClientID,
		"exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(),
	}
	for k, v := range issued.claims {
		allClaims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, allClaims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(f.key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "fake-access-token",
		"token_type":   "Bearer",
		"id_token":     signed,
		"expires_in":   300,
	})
}
