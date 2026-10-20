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

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected exactly one cookie, got %d", len(cookies))
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookies[0].Name, Value: cookies[0].Value + "tampered"})
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
