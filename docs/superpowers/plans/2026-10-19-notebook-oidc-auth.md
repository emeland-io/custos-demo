# Notebook App — OIDC/BFF Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stub auth middleware from Plan 1 with real OIDC
authentication: the Go backend performs the Authorization Code + PKCE
flow as a confidential client (BFF pattern), issues its own encrypted
session cookie to the SPA, maps a configurable claim to the `admin`
role, and upserts the local `users` row on every login.

**Architecture:** `internal/config` loads OIDC issuer/client and session
key settings from environment variables. `internal/auth` holds the
`oidc`/`oauth2` provider wrapper, a PKCE/state transient-cookie codec
used only across the login→callback round trip, the long-lived session
cookie codec, the login/callback/logout handlers, and the session
middleware that replaces `api.StubAuthMiddleware`. No handler from Plan
1 changes — they all consume `api.CurrentUser(ctx)`, which this plan's
middleware populates the same way the stub did.

**Tech Stack:** `github.com/coreos/go-oidc/v3`, `golang.org/x/oauth2`,
`github.com/gorilla/securecookie`, `github.com/golang-jwt/jwt/v5` (test-only,
to run a fake OIDC provider in-process for integration-style tests
without a live IdP).

**Spec:** `docs/superpowers/specs/2026-10-19-notebook-app.md`

## Handoff Note (read before starting)

This plan assumes Plan 1 (`docs/superpowers/plans/2026-10-19-notebook-backend-foundations.md`)
was implemented exactly as written: `api.CurrentUser`, `api.WithUser`,
`api.StubAuthMiddleware`, `api.NewRouter`'s signature, `repo.UserRepo`,
and `models.User`/`models.Role`. **Before this plan's first task starts,
the reviewer who signed off on Plan 1's final whole-branch review must
re-read this plan against the actual Plan 1 codebase** (not this plan's
assumptions) and fix any drift — renamed functions, changed signatures,
moved files — in the text below. Do not skip this if Plan 1's review
requested any changes to those specific names/signatures.

## Global Constraints

- All OIDC/session configuration comes from environment variables, read
  once at startup via `config.Load()` — no handler reads `os.Getenv`
  directly.
- The SPA never receives the OIDC ID/access token. Only an encrypted,
  HttpOnly, `SameSite=Lax` session cookie crosses to the browser.
- Session and PKCE-flow cookies are both encrypted+authenticated via
  `gorilla/securecookie` (AES + HMAC) — never a bare signed value.
- The transient PKCE/state cookie expires in 10 minutes and is cleared
  on successful callback; it is never reused across login attempts.
- `/healthz`, `/metrics`, and `/auth/*` are never behind the session
  middleware; everything under `/api/*` always is.
- Role is derived fresh from claims on every login — never edited via
  any API in this plan.

## Review Focus

- Callback request with a `state` value that doesn't match the one in
  the transient cookie (forged/replayed callback) — must be rejected
  with 400, never silently accepted.
- Callback request after the transient cookie has expired or is
  missing (user waited too long, or hit `/auth/callback` directly) —
  must be rejected with 400, not a panic on a nil cookie.
- An authenticated session cookie whose referenced user ID no longer
  exists in the `users` table (e.g. hand-edited DB) — middleware must
  return 401, not panic on a nil `*models.User` passed to a handler.
- A request to any `/api/*` route with no session cookie at all — must
  get 401 JSON, not a redirect loop or a 500.
- An admin-claim value that's a JSON array containing the admin group
  (common for an OIDC `groups` claim) vs. a single string — both forms
  must correctly produce `models.RoleAdmin`.

---

## File Structure

```
internal/config/config.go
internal/auth/session.go
internal/auth/flowcookie.go
internal/auth/oidc.go
internal/auth/role.go
internal/auth/handlers.go
internal/auth/middleware.go
internal/auth/fakeoidc_test.go
```

---

### Task 1: Configuration loading

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.OIDCConfig{IssuerURL, ClientID, ClientSecret, RedirectURL, AdminClaim, AdminClaimValue string}`; `config.SessionConfig{HashKey, BlockKey []byte}`; `config.Config{OIDC OIDCConfig; Session SessionConfig; DatabaseURL string}`; `config.Load() (*Config, error)`.

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config_test

import (
	"testing"

	"notebook/internal/config"
)

func TestLoad_AllRequiredVarsSet_Succeeds(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "https://idp.example.com")
	t.Setenv("OIDC_CLIENT_ID", "client-id")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://app.example.com/auth/callback")
	t.Setenv("SESSION_HASH_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_BLOCK_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "postgres://x")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.OIDC.IssuerURL != "https://idp.example.com" {
		t.Fatalf("unexpected issuer url: %s", cfg.OIDC.IssuerURL)
	}
	if cfg.OIDC.AdminClaim != "groups" {
		t.Fatalf("expected default admin claim 'groups', got %s", cfg.OIDC.AdminClaim)
	}
	if cfg.OIDC.AdminClaimValue != "notebook-admin" {
		t.Fatalf("expected default admin claim value, got %s", cfg.OIDC.AdminClaimValue)
	}
}

func TestLoad_MissingOIDCVars_Fails(t *testing.T) {
	t.Setenv("SESSION_HASH_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_BLOCK_KEY", "0123456789abcdef0123456789abcdef")

	if _, err := config.Load(); err == nil {
		t.Fatalf("expected error when OIDC vars are missing")
	}
}

func TestLoad_MissingSessionKeys_Fails(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "https://idp.example.com")
	t.Setenv("OIDC_CLIENT_ID", "client-id")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://app.example.com/auth/callback")

	if _, err := config.Load(); err == nil {
		t.Fatalf("expected error when session keys are missing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -v`
Expected: FAIL — package `config` undefined.

- [ ] **Step 3: Implement config loading**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
)

type OIDCConfig struct {
	IssuerURL       string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	AdminClaim      string
	AdminClaimValue string
}

type SessionConfig struct {
	HashKey  []byte
	BlockKey []byte
}

type Config struct {
	OIDC        OIDCConfig
	Session     SessionConfig
	DatabaseURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		OIDC: OIDCConfig{
			IssuerURL:       os.Getenv("OIDC_ISSUER_URL"),
			ClientID:        os.Getenv("OIDC_CLIENT_ID"),
			ClientSecret:    os.Getenv("OIDC_CLIENT_SECRET"),
			RedirectURL:     os.Getenv("OIDC_REDIRECT_URL"),
			AdminClaim:      envOr("OIDC_ADMIN_CLAIM", "groups"),
			AdminClaimValue: envOr("OIDC_ADMIN_CLAIM_VALUE", "notebook-admin"),
		},
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if cfg.OIDC.IssuerURL == "" || cfg.OIDC.ClientID == "" || cfg.OIDC.ClientSecret == "" || cfg.OIDC.RedirectURL == "" {
		return nil, fmt.Errorf("OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, and OIDC_REDIRECT_URL must all be set")
	}

	hashKey := os.Getenv("SESSION_HASH_KEY")
	blockKey := os.Getenv("SESSION_BLOCK_KEY")
	if hashKey == "" || blockKey == "" {
		return nil, fmt.Errorf("SESSION_HASH_KEY and SESSION_BLOCK_KEY must both be set")
	}
	cfg.Session.HashKey = []byte(hashKey)
	cfg.Session.BlockKey = []byte(blockKey)

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat: add OIDC and session configuration loading"
```

---

### Task 2: Session cookie codec

**Files:**
- Create: `internal/auth/session.go`
- Test: `internal/auth/session_test.go`

**Interfaces:**
- Consumes: `config.SessionConfig` (Task 1).
- Produces: `auth.NewSessionCodec(hashKey, blockKey []byte) *SessionCodec`; `(*SessionCodec).SetSession(w http.ResponseWriter, userID int64, ttl time.Duration) error`; `(*SessionCodec).ReadSession(r *http.Request) (userID int64, ok bool)`; `(*SessionCodec).ClearSession(w http.ResponseWriter)`.

- [ ] **Step 1: Add dependency**

```bash
go get github.com/gorilla/securecookie@v1.1.2
```

- [ ] **Step 2: Write the failing test**

```go
// internal/auth/session_test.go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"notebook/internal/auth"
)

func testCodec() *auth.SessionCodec {
	return auth.NewSessionCodec(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef0123456789abcdef"),
	)
}

func TestSessionCodec_RoundTrip(t *testing.T) {
	codec := testCodec()
	rec := httptest.NewRecorder()

	if err := codec.SetSession(rec, 42, time.Hour); err != nil {
		t.Fatalf("set session failed: %v", err)
	}

	req := &http.Request{Header: http.Header{"Cookie": rec.Header()["Set-Cookie"]}}
	userID, ok := codec.ReadSession(req)
	if !ok {
		t.Fatalf("expected session to be readable")
	}
	if userID != 42 {
		t.Fatalf("expected userID 42, got %d", userID)
	}
}

func TestSessionCodec_ExpiredSession_IsRejected(t *testing.T) {
	codec := testCodec()
	rec := httptest.NewRecorder()

	if err := codec.SetSession(rec, 42, -time.Hour); err != nil {
		t.Fatalf("set session failed: %v", err)
	}

	req := &http.Request{Header: http.Header{"Cookie": rec.Header()["Set-Cookie"]}}
	if _, ok := codec.ReadSession(req); ok {
		t.Fatalf("expected expired session to be rejected")
	}
}

func TestSessionCodec_TamperedCookie_IsRejected(t *testing.T) {
	codec := testCodec()
	rec := httptest.NewRecorder()
	if err := codec.SetSession(rec, 42, time.Hour); err != nil {
		t.Fatalf("set session failed: %v", err)
	}

	tampered := rec.Header()["Set-Cookie"][0] + "tampered"
	req := &http.Request{Header: http.Header{"Cookie": []string{tampered}}}
	if _, ok := codec.ReadSession(req); ok {
		t.Fatalf("expected tampered cookie to be rejected")
	}
}

func TestSessionCodec_MissingCookie_IsRejected(t *testing.T) {
	codec := testCodec()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := codec.ReadSession(req); ok {
		t.Fatalf("expected missing cookie to be rejected")
	}
}

func TestSessionCodec_ClearSession_ExpiresImmediately(t *testing.T) {
	codec := testCodec()
	rec := httptest.NewRecorder()
	codec.ClearSession(rec)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("expected a cookie with negative MaxAge, got %+v", cookies)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/auth/... -run TestSessionCodec -v`
Expected: FAIL — package `auth` undefined.

- [ ] **Step 4: Implement the session codec**

```go
// internal/auth/session.go
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/auth/... -run TestSessionCodec -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/auth/session.go internal/auth/session_test.go go.mod go.sum
git commit -m "feat: add encrypted session cookie codec"
```

---

### Task 3: PKCE/state transient flow cookie

**Files:**
- Create: `internal/auth/flowcookie.go`
- Test: `internal/auth/flowcookie_test.go`

**Interfaces:**
- Produces: `auth.NewFlowCodec(hashKey, blockKey []byte) *FlowCodec`; `(*FlowCodec).SetFlow(w http.ResponseWriter, state, verifier string) error`; `(*FlowCodec).ReadFlow(r *http.Request) (state, verifier string, ok bool)`; `(*FlowCodec).ClearFlow(w http.ResponseWriter)`; `auth.NewPKCEVerifier() (verifier string, err error)`; `auth.PKCEChallenge(verifier string) string`; `auth.NewState() (string, error)`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/auth/flowcookie_test.go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"notebook/internal/auth"
)

func testFlowCodec() *auth.FlowCodec {
	return auth.NewFlowCodec(
		[]byte("fedcba9876543210fedcba9876543210"),
		[]byte("fedcba9876543210fedcba9876543210"),
	)
}

func TestFlowCodec_RoundTrip(t *testing.T) {
	codec := testFlowCodec()
	rec := httptest.NewRecorder()

	if err := codec.SetFlow(rec, "state-123", "verifier-456"); err != nil {
		t.Fatalf("set flow failed: %v", err)
	}

	req := &http.Request{Header: http.Header{"Cookie": rec.Header()["Set-Cookie"]}}
	state, verifier, ok := codec.ReadFlow(req)
	if !ok || state != "state-123" || verifier != "verifier-456" {
		t.Fatalf("expected state/verifier round trip, got state=%q verifier=%q ok=%v", state, verifier, ok)
	}
}

func TestFlowCodec_MissingCookie_IsRejected(t *testing.T) {
	codec := testFlowCodec()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, _, ok := codec.ReadFlow(req); ok {
		t.Fatalf("expected missing flow cookie to be rejected")
	}
}

func TestNewPKCEVerifier_And_Challenge_AreConsistent(t *testing.T) {
	verifier, err := auth.NewPKCEVerifier()
	if err != nil {
		t.Fatalf("failed to generate verifier: %v", err)
	}
	if len(verifier) < 43 {
		t.Fatalf("expected verifier of at least 43 chars per RFC 7636, got %d", len(verifier))
	}

	challenge1 := auth.PKCEChallenge(verifier)
	challenge2 := auth.PKCEChallenge(verifier)
	if challenge1 != challenge2 {
		t.Fatalf("expected deterministic challenge for the same verifier")
	}
	if challenge1 == verifier {
		t.Fatalf("expected challenge to differ from verifier")
	}
}

func TestNewState_ProducesUniqueValues(t *testing.T) {
	s1, err := auth.NewState()
	if err != nil {
		t.Fatalf("failed to generate state: %v", err)
	}
	s2, err := auth.NewState()
	if err != nil {
		t.Fatalf("failed to generate state: %v", err)
	}
	if s1 == s2 {
		t.Fatalf("expected two calls to NewState to produce different values")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth/... -run "TestFlowCodec|TestNewPKCEVerifier|TestNewState" -v`
Expected: FAIL — `auth.FlowCodec`, `auth.NewPKCEVerifier`, `auth.NewState` undefined.

- [ ] **Step 3: Implement PKCE/state helpers and the flow cookie codec**

```go
// internal/auth/flowcookie.go
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
const flowTTL = 10 * time.Minute

type flowData struct {
	State    string
	Verifier string
}

type FlowCodec struct {
	sc *securecookie.SecureCookie
}

func NewFlowCodec(hashKey, blockKey []byte) *FlowCodec {
	return &FlowCodec{sc: securecookie.New(hashKey, blockKey)}
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/... -run "TestFlowCodec|TestNewPKCEVerifier|TestNewState" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/auth/flowcookie.go internal/auth/flowcookie_test.go
git commit -m "feat: add PKCE verifier/challenge and state helpers with transient flow cookie"
```

---

### Task 4: Role extraction from OIDC claims

**Files:**
- Create: `internal/auth/role.go`
- Test: `internal/auth/role_test.go`

**Interfaces:**
- Consumes: `models.Role`, `models.RoleUser`, `models.RoleAdmin` (Plan 1).
- Produces: `auth.ExtractRole(claims map[string]any, adminClaim, adminValue string) models.Role`.

- [ ] **Step 1: Write the failing test**

```go
// internal/auth/role_test.go
package auth_test

import (
	"testing"

	"notebook/internal/auth"
	"notebook/internal/models"
)

func TestExtractRole_StringClaimMatches(t *testing.T) {
	claims := map[string]any{"groups": "notebook-admin"}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleAdmin {
		t.Fatalf("expected admin role, got %s", got)
	}
}

func TestExtractRole_ArrayClaimContainsValue(t *testing.T) {
	claims := map[string]any{"groups": []any{"other-group", "notebook-admin"}}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleAdmin {
		t.Fatalf("expected admin role, got %s", got)
	}
}

func TestExtractRole_ArrayClaimWithoutValue_IsUser(t *testing.T) {
	claims := map[string]any{"groups": []any{"other-group"}}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleUser {
		t.Fatalf("expected user role, got %s", got)
	}
}

func TestExtractRole_MissingClaim_IsUser(t *testing.T) {
	claims := map[string]any{}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleUser {
		t.Fatalf("expected user role, got %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/... -run TestExtractRole -v`
Expected: FAIL — `auth.ExtractRole` undefined.

- [ ] **Step 3: Implement role extraction**

```go
// internal/auth/role.go
package auth

import "notebook/internal/models"

func ExtractRole(claims map[string]any, adminClaim, adminValue string) models.Role {
	val, ok := claims[adminClaim]
	if !ok {
		return models.RoleUser
	}

	switch v := val.(type) {
	case string:
		if v == adminValue {
			return models.RoleAdmin
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == adminValue {
				return models.RoleAdmin
			}
		}
	}
	return models.RoleUser
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/... -run TestExtractRole -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/auth/role.go internal/auth/role_test.go
git commit -m "feat: add OIDC claim-to-role extraction"
```

---

### Task 5: Fake OIDC test server (test-only integration harness)

**Files:**
- Create: `internal/auth/fakeoidc_test.go`

**Interfaces:**
- Produces (test-only, package `auth_test`): `newFakeOIDCServer(t *testing.T) *fakeOIDCServer` with fields `IssuerURL`, `ClientID`, `ClientSecret` and method `IssueCode(claims map[string]any) string`.

This task has no separate "failing test" step — it is pure test
infrastructure consumed by Task 6. Its own correctness is verified by
Task 6's tests passing against it.

- [ ] **Step 1: Add the JWT dependency used only to sign fake ID tokens**

```bash
go get github.com/golang-jwt/jwt/v5@v5.2.1
```

- [ ] **Step 2: Implement the fake OIDC server**

```go
// internal/auth/fakeoidc_test.go
package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
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
	issuedCodes map[string]map[string]any
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
		issuedCodes:  map[string]map[string]any{},
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
		"issuer":                                 f.IssuerURL,
		"authorization_endpoint":                 f.IssuerURL + "/authorize",
		"token_endpoint":                         f.IssuerURL + "/token",
		"jwks_uri":                               f.IssuerURL + "/keys",
		"id_token_signing_alg_values_supported":  []string{"RS256"},
		"userinfo_endpoint":                      f.IssuerURL + "/userinfo",
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

// IssueCode registers claims for a fresh, single-use authorization
// code that /token will exchange for a signed ID token containing
// exactly those claims (plus iss/aud/exp/iat).
func (f *fakeOIDCServer) IssueCode(claims map[string]any) string {
	code := fmt.Sprintf("code-%d", len(f.issuedCodes)+1)
	f.issuedCodes[code] = claims
	return code
}

func (f *fakeOIDCServer) handleToken(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	claims, ok := f.issuedCodes[code]
	if !ok {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
		return
	}
	delete(f.issuedCodes, code)

	now := time.Now()
	allClaims := jwt.MapClaims{
		"iss": f.IssuerURL,
		"aud": f.ClientID,
		"exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(),
	}
	for k, v := range claims {
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
```

- [ ] **Step 3: Commit**

```bash
git add internal/auth/fakeoidc_test.go go.mod go.sum
git commit -m "test: add fake in-process OIDC provider for auth integration tests"
```

---

### Task 6: OIDC provider wrapper, login handler, callback handler

**Files:**
- Create: `internal/auth/oidc.go`
- Create: `internal/auth/handlers.go`
- Test: `internal/auth/handlers_test.go`

**Interfaces:**
- Consumes: `fakeOIDCServer` (Task 5), `FlowCodec`/`SessionCodec` (Tasks 2–3), `ExtractRole` (Task 4), `repo.UserRepo` (Plan 1).
- Produces: `auth.NewProvider(ctx context.Context, cfg config.OIDCConfig) (*Provider, error)`; `auth.NewLoginHandler(provider *Provider, flowCodec *FlowCodec) http.HandlerFunc`; `auth.NewCallbackHandler(provider *Provider, flowCodec *FlowCodec, sessionCodec *SessionCodec, users *repo.UserRepo, adminClaim, adminValue, postLoginRedirect string) http.HandlerFunc`.

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/coreos/go-oidc/v3@v3.11.0
go get golang.org/x/oauth2@v0.23.0
```

- [ ] **Step 2: Write the failing integration-style tests**

```go
// internal/auth/handlers_test.go
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

	// --- Simulate the IdP issuing a code for this flow's claims ---
	code := fake.IssueCode(map[string]any{
		"sub":    "user-sub-1",
		"email":  "alice@example.com",
		"name":   "Alice",
		"groups": []any{"notebook-admin"},
	})

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
```

`authTestDSN(t)` reuses the same testcontainers pattern as
`internal/repo`'s `newTestPool` but lives in this package — add it as a
small helper in a new `internal/auth/testutil_test.go` file that starts
a `postgres:16-alpine` container exactly like
`internal/repo/testutil_test.go` does and returns its connection string
(DSN) instead of a connected pool, since `newAuthTestUserRepo` above
calls `db.Connect` itself. Copy the container-startup block verbatim
from `internal/repo/testutil_test.go`, replacing the final
`db.Connect`+return-pool lines with `return container.ConnectionString(ctx, "sslmode=disable")`
(propagating its error via `t.Fatalf` as the existing helper does).

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/auth/... -run "TestLoginThenCallback|TestCallback" -v`
Expected: FAIL — `auth.NewProvider`, `auth.NewLoginHandler`,
`auth.NewCallbackHandler` undefined.

- [ ] **Step 4: Implement the OIDC provider wrapper**

```go
// internal/auth/oidc.go
package auth

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"notebook/internal/config"
)

type Provider struct {
	OAuth2Config *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

func NewProvider(ctx context.Context, cfg config.OIDCConfig) (*Provider, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, err
	}

	return &Provider{
		OAuth2Config: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		Verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}
```

- [ ] **Step 5: Implement the login and callback handlers**

```go
// internal/auth/handlers.go
package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"notebook/internal/repo"
)

const sessionTTL = 24 * time.Hour

func NewLoginHandler(provider *Provider, flowCodec *FlowCodec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := NewState()
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to start login")
			return
		}
		verifier, err := NewPKCEVerifier()
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to start login")
			return
		}

		if err := flowCodec.SetFlow(w, state, verifier); err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to start login")
			return
		}

		authURL := provider.OAuth2Config.AuthCodeURL(state,
			oauth2.SetAuthURLParam("code_challenge", PKCEChallenge(verifier)),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

func NewCallbackHandler(
	provider *Provider,
	flowCodec *FlowCodec,
	sessionCodec *SessionCodec,
	users *repo.UserRepo,
	adminClaim, adminValue, postLoginRedirect string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedState, verifier, ok := flowCodec.ReadFlow(r)
		if !ok {
			writeAuthError(w, http.StatusBadRequest, "login session expired or missing, please try again")
			return
		}
		flowCodec.ClearFlow(w)

		if r.URL.Query().Get("state") != expectedState {
			writeAuthError(w, http.StatusBadRequest, "state mismatch")
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			writeAuthError(w, http.StatusBadRequest, "missing authorization code")
			return
		}

		token, err := provider.OAuth2Config.Exchange(r.Context(), code,
			oauth2.SetAuthURLParam("code_verifier", verifier),
		)
		if err != nil {
			writeAuthError(w, http.StatusBadRequest, "failed to exchange authorization code")
			return
		}

		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok {
			writeAuthError(w, http.StatusBadGateway, "identity provider did not return an id_token")
			return
		}

		idToken, err := provider.Verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			writeAuthError(w, http.StatusBadGateway, "failed to verify id_token")
			return
		}

		var claims map[string]any
		if err := idToken.Claims(&claims); err != nil {
			writeAuthError(w, http.StatusBadGateway, "failed to parse id_token claims")
			return
		}

		subject, _ := claims["sub"].(string)
		email, _ := claims["email"].(string)
		displayName, _ := claims["name"].(string)
		if displayName == "" {
			displayName = email
		}
		role := ExtractRole(claims, adminClaim, adminValue)

		user, err := users.UpsertByOIDCSubject(r.Context(), subject, email, displayName, role)
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to persist user")
			return
		}

		if err := sessionCodec.SetSession(w, user.ID, sessionTTL); err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to create session")
			return
		}

		http.Redirect(w, r, postLoginRedirect, http.StatusFound)
	}
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
```

- [ ] **Step 6: Add the auth-package testcontainers DSN helper**

```go
// internal/auth/testutil_test.go
package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func authTestDSN(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("notebook"),
		postgres.WithUsername("notebook"),
		postgres.WithPassword("notebook"),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}
	return dsn
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/auth/... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/auth go.mod go.sum
git commit -m "feat: add OIDC provider wrapper and login/callback handlers"
```

---

### Task 7: Session middleware, logout handler, and router wiring

**Files:**
- Create: `internal/auth/middleware.go`
- Modify: `internal/api/router.go`
- Modify: `cmd/server/main.go`
- Test: `internal/auth/middleware_test.go`

**Interfaces:**
- Consumes: `SessionCodec` (Task 2), `api.WithUser`/`api.CurrentUser` (Plan 1), `repo.UserRepo` (Plan 1).
- Produces: `auth.NewSessionMiddleware(sessionCodec *SessionCodec, users *repo.UserRepo) func(http.Handler) http.Handler`; `auth.NewLogoutHandler(sessionCodec *SessionCodec) http.HandlerFunc`.

- [ ] **Step 1: Write the failing middleware tests**

```go
// internal/auth/middleware_test.go
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"notebook/internal/api"
	"notebook/internal/auth"
	"notebook/internal/models"
)

func TestSessionMiddleware_ValidSession_InjectsUser(t *testing.T) {
	users := newAuthTestUserRepo(t)
	created, err := users.UpsertByOIDCSubject(context.Background(), "mw-user", "mw@example.com", "MW User", models.RoleUser)
	if err != nil {
		t.Fatalf("seed user failed: %v", err)
	}

	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))
	setRec := httptest.NewRecorder()
	if err := sessionCodec.SetSession(setRec, created.ID, time.Hour); err != nil {
		t.Fatalf("set session failed: %v", err)
	}

	var gotUser *models.User
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _ = api.CurrentUser(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.NewSessionMiddleware(sessionCodec, users)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
	req.Header.Set("Cookie", setRec.Header().Get("Set-Cookie"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotUser == nil || gotUser.ID != created.ID {
		t.Fatalf("expected injected user with ID %d, got %+v", created.ID, gotUser)
	}
}

func TestSessionMiddleware_NoSession_Returns401(t *testing.T) {
	users := newAuthTestUserRepo(t)
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next handler should not be called without a valid session")
	})
	handler := auth.NewSessionMiddleware(sessionCodec, users)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSessionMiddleware_SessionForDeletedUser_Returns401(t *testing.T) {
	users := newAuthTestUserRepo(t)
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))

	setRec := httptest.NewRecorder()
	const nonExistentUserID = int64(999999)
	if err := sessionCodec.SetSession(setRec, nonExistentUserID, time.Hour); err != nil {
		t.Fatalf("set session failed: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next handler should not be called for a session with no backing user")
	})
	handler := auth.NewSessionMiddleware(sessionCodec, users)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
	req.Header.Set("Cookie", setRec.Header().Get("Set-Cookie"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a session pointing at a nonexistent user, got %d", rec.Code)
	}
}

func TestLogoutHandler_ClearsSessionCookie(t *testing.T) {
	sessionCodec := auth.NewSessionCodec([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))
	handler := auth.NewLogoutHandler(sessionCodec)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("expected a cleared cookie, got %+v", cookies)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth/... -run "TestSessionMiddleware|TestLogoutHandler" -v`
Expected: FAIL — `auth.NewSessionMiddleware`, `auth.NewLogoutHandler` undefined.

- [ ] **Step 3: Implement the session middleware and logout handler**

```go
// internal/auth/middleware.go
package auth

import (
	"net/http"

	"notebook/internal/api"
	"notebook/internal/repo"
)

func NewSessionMiddleware(sessionCodec *SessionCodec, users *repo.UserRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := sessionCodec.ReadSession(r)
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			user, err := users.GetByID(r.Context(), userID)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			ctx := api.WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func NewLogoutHandler(sessionCodec *SessionCodec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionCodec.ClearSession(w)
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/... -v`
Expected: PASS

- [ ] **Step 5: Wire everything into the router and main.go**

```go
// internal/api/router.go — extend NewRouter's signature to accept the
// auth middleware and the auth handlers as http.HandlerFunc values,
// so internal/api never imports internal/auth (avoiding an import
// cycle, since internal/auth imports internal/api for WithUser/CurrentUser).
package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/metrics"
	"notebook/internal/repo"
)

type AuthWiring struct {
	SessionMiddleware func(http.Handler) http.Handler
	LoginHandler      http.HandlerFunc
	CallbackHandler   http.HandlerFunc
	LogoutHandler     http.HandlerFunc
}

func NewRouter(logger *slog.Logger, notes *repo.NoteRepo, comments *repo.CommentRepo, auth AuthWiring) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)
	r.Handle("/metrics", metrics.Handler()).Methods(http.MethodGet)

	r.HandleFunc("/auth/login", auth.LoginHandler).Methods(http.MethodGet)
	r.HandleFunc("/auth/callback", auth.CallbackHandler).Methods(http.MethodGet)
	r.HandleFunc("/auth/logout", auth.LogoutHandler).Methods(http.MethodPost)

	api := r.PathPrefix("/api").Subrouter()
	api.Use(auth.SessionMiddleware)
	api.HandleFunc("/render-preview", RenderPreviewHandler).Methods(http.MethodPost)

	notesHandler := NewNotesHandler(notes)
	api.HandleFunc("/notes", notesHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/notes", notesHandler.Create).Methods(http.MethodPost)
	api.HandleFunc("/notes/{id}", notesHandler.Get).Methods(http.MethodGet)
	api.HandleFunc("/notes/{id}", notesHandler.Update).Methods(http.MethodPut)
	api.HandleFunc("/notes/{id}", notesHandler.Delete).Methods(http.MethodDelete)

	commentsHandler := NewCommentsHandler(comments)
	api.HandleFunc("/notes/{noteId}/comments", commentsHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/notes/{noteId}/comments", commentsHandler.Create).Methods(http.MethodPost)
	api.HandleFunc("/comments/{id}", commentsHandler.Update).Methods(http.MethodPut)
	api.HandleFunc("/comments/{id}", commentsHandler.Delete).Methods(http.MethodDelete)

	return metrics.Middleware(r)
}
```

```go
// cmd/server/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"

	"notebook/internal/api"
	"notebook/internal/auth"
	"notebook/internal/config"
	"notebook/internal/db"
	"notebook/internal/db/migrations"
	"notebook/internal/logging"
	"notebook/internal/repo"
)

func main() {
	logger := logging.New()
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", slog.String("error", err.Error()))
		return
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		return
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		logger.Error("failed to run migrations", slog.String("error", err.Error()))
		return
	}

	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	provider, err := auth.NewProvider(ctx, cfg.OIDC)
	if err != nil {
		logger.Error("failed to initialize OIDC provider", slog.String("error", err.Error()))
		return
	}
	flowCodec := auth.NewFlowCodec(cfg.Session.HashKey, cfg.Session.BlockKey)
	sessionCodec := auth.NewSessionCodec(cfg.Session.HashKey, cfg.Session.BlockKey)

	router := api.NewRouter(logger, notes, comments, api.AuthWiring{
		SessionMiddleware: auth.NewSessionMiddleware(sessionCodec, users),
		LoginHandler:      auth.NewLoginHandler(provider, flowCodec),
		CallbackHandler:   auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, cfg.OIDC.AdminClaim, cfg.OIDC.AdminClaimValue, "/"),
		LogoutHandler:     auth.NewLogoutHandler(sessionCodec),
	})

	logger.Info("starting server", slog.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", router); err != nil {
		logger.Error("server stopped", slog.String("error", err.Error()))
	}
}
```

Since `NewRouter`'s signature changed, update every test in
`internal/api` that calls `NewRouter` directly (Plan 1's handler tests
built their own `mux.Router` manually with `StubAuthMiddleware`, so
they are unaffected — only `cmd/server/main.go`'s call site changes).

- [ ] **Step 6: Run the full test suite to verify everything still passes**

Run: `go test ./... -v`
Expected: PASS (all packages)

- [ ] **Step 7: Commit**

```bash
git add internal/auth internal/api/router.go cmd/server/main.go
git commit -m "feat: wire session middleware and OIDC login/callback/logout into the router"
```

---

## Out of scope for this plan (covered by later plans)

- Vue frontend login UX (redirect-to-`/auth/login`, handling 401s).
- Helm values for `OIDC_*`/`SESSION_*` env vars and their Secret refs.
- End-to-end testing against a real Dex instance (Plan 6) — this plan's
  tests use an in-process fake IdP for speed and isolation; the e2e
  suite is what proves the flow against a real OIDC implementation.

## Handoff to Plan 3

Plan 3 (`docs/superpowers/plans/2026-10-19-notebook-frontend.md`) assumes
the backend exposes `GET /auth/login`, `GET /auth/callback`, and
`POST /auth/logout`, that unauthenticated `/api/*` requests return 401
JSON (not a redirect), and that the session cookie is `HttpOnly` (so the
frontend cannot and must not try to read it directly — it just calls
`/api/*` with `credentials: 'include'` and reacts to 401s by navigating
to `/auth/login`). Once this plan's tasks are complete and its final
whole-branch review has passed, **the reviewer who signed off on that
review must read Plan 3 against what was actually built** and correct
any drift in route paths, status codes, or cookie behavior before Plan
3 execution begins.
