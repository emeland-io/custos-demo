package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gorilla/mux"

	"notebook/internal/models"
	"notebook/internal/repo"
)

// newTestServer mirrors internal/repo's testcontainers setup so handler
// tests exercise the real repository stack, not mocks.
func newTestServer(t *testing.T, asUser *models.User) (*mux.Router, *repo.NoteRepo, *models.User) {
	t.Helper()
	pool := newAPITestPool(t)
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)

	author, err := users.UpsertByOIDCSubject(context.Background(), "handler-author", "a@example.com", "Author", models.RoleUser)
	if err != nil {
		t.Fatalf("seed author failed: %v", err)
	}
	user := asUser
	if user == nil {
		user = author
	}

	router := mux.NewRouter()
	router.Use(StubAuthMiddleware(user))
	handler := NewNotesHandler(notes)
	router.HandleFunc("/api/notes", handler.List).Methods(http.MethodGet)
	router.HandleFunc("/api/notes", handler.Create).Methods(http.MethodPost)
	router.HandleFunc("/api/notes/{id}", handler.Get).Methods(http.MethodGet)
	router.HandleFunc("/api/notes/{id}", handler.Update).Methods(http.MethodPut)
	router.HandleFunc("/api/notes/{id}", handler.Delete).Methods(http.MethodDelete)

	return router, notes, author
}

func TestNotesHandler_CreateRejectsBlankTitle(t *testing.T) {
	router, _, _ := newTestServer(t, nil)

	body, _ := json.Marshal(map[string]string{"title": "   ", "content": "content"})
	req := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank title, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotesHandler_CreateThenGet(t *testing.T) {
	router, _, _ := newTestServer(t, nil)

	body, _ := json.Marshal(map[string]string{"title": "My Note", "content": "Hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/notes/"+itoa(created.ID), nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
}

func TestNotesHandler_UpdateByNonAuthorIsForbidden(t *testing.T) {
	router, notes, author := newTestServer(t, nil)
	note, err := notes.Create(context.Background(), "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("seed note failed: %v", err)
	}

	intruder := &models.User{ID: author.ID + 999, Role: models.RoleUser}
	router.Use(StubAuthMiddleware(intruder))

	body, _ := json.Marshal(map[string]string{"title": "Hacked", "content": "Hacked"})
	req := httptest.NewRequest(http.MethodPut, "/api/notes/"+itoa(note.ID), bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
