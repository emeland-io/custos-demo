package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"notebook/internal/metrics"
)

func TestMiddleware_RecordsRequestCount(t *testing.T) {
	handler := metrics.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if !strings.Contains(metricsRec.Body.String(), "http_requests_total") {
		t.Fatalf("expected http_requests_total metric to be present, got body:\n%s", metricsRec.Body.String())
	}
}

func TestBusinessCounters_AreRegistered(t *testing.T) {
	metrics.NotesCreatedTotal.Inc()
	metrics.CommentsCreatedTotal.Inc()
	metrics.CommentsDeletedTotal.Inc()

	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	for _, name := range []string{"notes_created_total", "comments_created_total", "comments_deleted_total"} {
		if !strings.Contains(rec.Body.String(), name) {
			t.Fatalf("expected %s to be present in metrics output", name)
		}
	}
}
