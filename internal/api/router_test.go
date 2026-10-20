package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"notebook/internal/metrics"
)

func TestNewRouter_MetricsLabelByRouteTemplate_NotUnmatched(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(logger, nil, nil, AuthWiring{
		SessionMiddleware: func(next http.Handler) http.Handler { return next },
		LoginHandler:      func(w http.ResponseWriter, r *http.Request) {},
		CallbackHandler:   func(w http.ResponseWriter, r *http.Request) {},
		LogoutHandler:     func(w http.ResponseWriter, r *http.Request) {},
	}, fstest.MapFS{"index.html": {Data: []byte("shell")}})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	metricsRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRec.Body.String()

	if !strings.Contains(body, `route="/healthz"`) {
		t.Fatalf(`expected a templated route="/healthz" label, got body:%s`, body)
	}
	if strings.Contains(body, `route="unmatched"`) {
		t.Fatalf(`expected no "unmatched" route label for a matched request, got body:%s`, body)
	}
}

func testRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(logger, nil, nil, AuthWiring{
		SessionMiddleware: func(next http.Handler) http.Handler { return next },
		LoginHandler:      func(w http.ResponseWriter, r *http.Request) {},
		CallbackHandler:   func(w http.ResponseWriter, r *http.Request) {},
		LogoutHandler:     func(w http.ResponseWriter, r *http.Request) {},
	}, fstest.MapFS{"index.html": {Data: []byte("shell")}})
}

func TestNewRouter_UnknownAPIPath_Returns404JSON_NotSPAShell(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/bogus", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "shell") {
		t.Fatalf("expected a JSON 404, got the SPA shell: %s", rec.Body.String())
	}
}

// gorilla/mux (v1.8.1) can lose a carried ErrMethodMismatch to a 404 once a
// path-variable sibling route (e.g. "/notes/{id}") is tried and its path
// regexp partially matches before failing — a library quirk, not something
// this router controls. Either way the response must be a JSON error, never
// the SPA shell, since that's what breaks the frontend's apiFetch() error
// handling (it would otherwise call response.json() on HTML).
func TestNewRouter_APIMethodMismatch_ReturnsJSONError_NotSPAShell(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodPatch, "/api/notes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 or 405, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "shell") {
		t.Fatalf("expected a JSON error, got the SPA shell: %s", rec.Body.String())
	}
}

func TestNewRouter_AuthMethodMismatch_Returns405_NotSPAShell(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_UnknownAuthPath_Returns404_NotSPAShell(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodGet, "/auth/bogus", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
