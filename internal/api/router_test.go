package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"notebook/internal/metrics"
)

func TestNewRouter_MetricsLabelByRouteTemplate_NotUnmatched(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(logger, nil, nil, AuthWiring{
		SessionMiddleware: func(next http.Handler) http.Handler { return next },
		LoginHandler:      func(w http.ResponseWriter, r *http.Request) {},
		CallbackHandler:   func(w http.ResponseWriter, r *http.Request) {},
		LogoutHandler:     func(w http.ResponseWriter, r *http.Request) {},
	})

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
