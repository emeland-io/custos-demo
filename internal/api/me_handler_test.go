package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"notebook/internal/models"
)

func TestMeHandler_ReturnsCurrentUser(t *testing.T) {
	user := &models.User{ID: 7, Email: "a@example.com", DisplayName: "Alice", Role: models.RoleAdmin}

	router := mux.NewRouter()
	router.Use(StubAuthMiddleware(user))
	router.HandleFunc("/api/me", MeHandler).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"id":7`, `"email":"a@example.com"`, `"display_name":"Alice"`, `"role":"admin"`} {
		if !contains(body, want) {
			t.Fatalf("expected body to contain %q, got %s", want, body)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
