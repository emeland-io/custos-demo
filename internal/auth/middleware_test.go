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
