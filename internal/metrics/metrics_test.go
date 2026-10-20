package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

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

func TestMiddleware_RegisteredViaMuxUse_LabelsByPathTemplateNotRawPath(t *testing.T) {
	router := mux.NewRouter()
	router.Use(metrics.Middleware)
	router.HandleFunc("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, id := range []string{"42", "43", "44"} {
		req := httptest.NewRequest(http.MethodGet, "/items/"+id, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}

	metricsRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRec.Body.String()

	if !strings.Contains(body, `route="/items/{id}"`) {
		t.Fatalf("expected a single templated route label, got body:\n%s", body)
	}
	for _, id := range []string{"42", "43", "44"} {
		if strings.Contains(body, `route="/items/`+id+`"`) {
			t.Fatalf("expected no per-id route label for id %s, got body:\n%s", id, body)
		}
	}
}
