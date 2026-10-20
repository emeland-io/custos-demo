package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"notebook/internal/auth"
	"notebook/internal/config"
	"notebook/internal/db"
	"notebook/internal/db/migrations"
	"notebook/internal/repo"
)

func newAuthTestUserRepo(t *testing.T) *repo.UserRepo {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Connect(ctx, authTestDSN(t))
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}
	return repo.NewUserRepo(pool)
}

func TestLoginThenCallback_CreatesSessionAndUpsertsUser(t *testing.T) {
	fake := newFakeOIDCServer(t)
	provider, err := auth.NewProvider(context.Background(), config.OIDCConfig{
		IssuerURL:    fake.IssuerURL,
		ClientID:     fake.ClientID,
		ClientSecret: fake.ClientSecret,
		RedirectURL:  "https://app.example.com/auth/callback",
	})
	if err != nil {
		t.Fatalf("failed to build provider: %v", err)
	}

	flowCodec := auth.NewFlowCodec([]byte("fedcba9876543210fedcba9876543210"), []byte("fedcba9876543210fedcba9876543210"))
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))
	users := newAuthTestUserRepo(t)

	// --- Login ---
	loginHandler := auth.NewLoginHandler(provider, flowCodec)
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	loginRec := httptest.NewRecorder()
	loginHandler(loginRec, loginReq)

	if loginRec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", loginRec.Code)
	}
	redirectURL, err := url.Parse(loginRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("failed to parse redirect URL: %v", err)
	}
	state := redirectURL.Query().Get("state")
	if state == "" {
		t.Fatalf("expected state query param on redirect")
	}
	codeChallenge := redirectURL.Query().Get("code_challenge")
	if codeChallenge == "" {
		t.Fatalf("expected code_challenge query param on redirect")
	}

	// --- Simulate the IdP issuing a code for this flow's claims ---
	code := fake.IssueCode(map[string]any{
		"sub":    "user-sub-1",
		"email":  "alice@example.com",
		"name":   "Alice",
		"groups": []any{"notebook-admin"},
	}, codeChallenge)

	// --- Callback ---
	callbackHandler := auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, "groups", "notebook-admin", "/")
	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code="+code, nil)
	callbackReq.Header.Set("Cookie", loginRec.Header().Get("Set-Cookie"))
	callbackRec := httptest.NewRecorder()
	callbackHandler(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect after callback, got %d: %s", callbackRec.Code, callbackRec.Body.String())
	}

	var sessionCookieHeader string
	for _, c := range callbackRec.Header()["Set-Cookie"] {
		sessionCookieHeader = c
	}
	sessionReq := &http.Request{Header: http.Header{"Cookie": []string{sessionCookieHeader}}}
	userID, ok := sessionCodec.ReadSession(sessionReq)
	if !ok {
		t.Fatalf("expected a valid session cookie to be set")
	}

	user, err := users.GetByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("expected upserted user to be fetchable: %v", err)
	}
	if user.Email != "alice@example.com" || user.OIDCSubject != "user-sub-1" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if user.Role != "admin" {
		t.Fatalf("expected admin role from groups claim, got %s", user.Role)
	}
}

func TestCallback_WrongState_IsRejected(t *testing.T) {
	fake := newFakeOIDCServer(t)
	provider, err := auth.NewProvider(context.Background(), config.OIDCConfig{
		IssuerURL:    fake.IssuerURL,
		ClientID:     fake.ClientID,
		ClientSecret: fake.ClientSecret,
		RedirectURL:  "https://app.example.com/auth/callback",
	})
	if err != nil {
		t.Fatalf("failed to build provider: %v", err)
	}

	flowCodec := auth.NewFlowCodec([]byte("fedcba9876543210fedcba9876543210"), []byte("fedcba9876543210fedcba9876543210"))
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))
	users := newAuthTestUserRepo(t)

	loginHandler := auth.NewLoginHandler(provider, flowCodec)
	loginRec := httptest.NewRecorder()
	loginHandler(loginRec, httptest.NewRequest(http.MethodGet, "/auth/login", nil))

	callbackHandler := auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, "groups", "notebook-admin", "/")
	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?state=wrong-state&code=anything", nil)
	callbackReq.Header.Set("Cookie", loginRec.Header().Get("Set-Cookie"))
	callbackRec := httptest.NewRecorder()
	callbackHandler(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for mismatched state, got %d", callbackRec.Code)
	}
}

func TestCallback_WrongPKCEVerifier_IsRejected(t *testing.T) {
	fake := newFakeOIDCServer(t)
	provider, err := auth.NewProvider(context.Background(), config.OIDCConfig{
		IssuerURL:    fake.IssuerURL,
		ClientID:     fake.ClientID,
		ClientSecret: fake.ClientSecret,
		RedirectURL:  "https://app.example.com/auth/callback",
	})
	if err != nil {
		t.Fatalf("failed to build provider: %v", err)
	}

	flowCodec := auth.NewFlowCodec([]byte("fedcba9876543210fedcba9876543210"), []byte("fedcba9876543210fedcba9876543210"))
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))
	users := newAuthTestUserRepo(t)

	loginHandler := auth.NewLoginHandler(provider, flowCodec)
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	loginRec := httptest.NewRecorder()
	loginHandler(loginRec, loginReq)
	redirectURL, err := url.Parse(loginRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("failed to parse redirect URL: %v", err)
	}
	state := redirectURL.Query().Get("state")

	// Issue the code against a challenge that does NOT match the
	// verifier stashed in this flow's cookie, simulating an
	// authorization code obtained for a different (or forged) flow.
	code := fake.IssueCode(map[string]any{
		"sub":    "user-sub-1",
		"email":  "alice@example.com",
		"groups": []any{"notebook-admin"},
	}, "wrong-challenge-does-not-match-verifier")

	callbackHandler := auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, "groups", "notebook-admin", "/")
	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code="+code, nil)
	callbackReq.Header.Set("Cookie", loginRec.Header().Get("Set-Cookie"))
	callbackRec := httptest.NewRecorder()
	callbackHandler(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when the code_verifier doesn't match the issued challenge, got %d: %s", callbackRec.Code, callbackRec.Body.String())
	}
}

func TestCallback_MissingSubClaim_IsRejected(t *testing.T) {
	fake := newFakeOIDCServer(t)
	provider, err := auth.NewProvider(context.Background(), config.OIDCConfig{
		IssuerURL:    fake.IssuerURL,
		ClientID:     fake.ClientID,
		ClientSecret: fake.ClientSecret,
		RedirectURL:  "https://app.example.com/auth/callback",
	})
	if err != nil {
		t.Fatalf("failed to build provider: %v", err)
	}

	flowCodec := auth.NewFlowCodec([]byte("fedcba9876543210fedcba9876543210"), []byte("fedcba9876543210fedcba9876543210"))
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))
	users := newAuthTestUserRepo(t)

	loginHandler := auth.NewLoginHandler(provider, flowCodec)
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	loginRec := httptest.NewRecorder()
	loginHandler(loginRec, loginReq)
	redirectURL, err := url.Parse(loginRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("failed to parse redirect URL: %v", err)
	}
	state := redirectURL.Query().Get("state")
	codeChallenge := redirectURL.Query().Get("code_challenge")

	// No "sub" claim at all - a misconfigured IdP or claim mapper.
	code := fake.IssueCode(map[string]any{
		"email": "no-subject@example.com",
	}, codeChallenge)

	callbackHandler := auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, "groups", "notebook-admin", "/")
	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code="+code, nil)
	callbackReq.Header.Set("Cookie", loginRec.Header().Get("Set-Cookie"))
	callbackRec := httptest.NewRecorder()
	callbackHandler(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for an id_token with no sub claim, got %d: %s", callbackRec.Code, callbackRec.Body.String())
	}
}

func TestCallback_MissingFlowCookie_IsRejected(t *testing.T) {
	fake := newFakeOIDCServer(t)
	provider, err := auth.NewProvider(context.Background(), config.OIDCConfig{
		IssuerURL:    fake.IssuerURL,
		ClientID:     fake.ClientID,
		ClientSecret: fake.ClientSecret,
		RedirectURL:  "https://app.example.com/auth/callback",
	})
	if err != nil {
		t.Fatalf("failed to build provider: %v", err)
	}

	flowCodec := auth.NewFlowCodec([]byte("fedcba9876543210fedcba9876543210"), []byte("fedcba9876543210fedcba9876543210"))
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))
	users := newAuthTestUserRepo(t)

	callbackHandler := auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, "groups", "notebook-admin", "/")
	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?state=s&code=c", nil)
	callbackRec := httptest.NewRecorder()
	callbackHandler(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when flow cookie is missing, got %d", callbackRec.Code)
	}
}
