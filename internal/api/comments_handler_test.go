package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"notebook/internal/models"
	"notebook/internal/repo"
)

func newCommentsTestServer(t *testing.T, asUser *models.User) (*mux.Router, *repo.NoteRepo, *repo.CommentRepo, *models.User) {
	t.Helper()
	pool := newAPITestPool(t)
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author, err := users.UpsertByOIDCSubject(context.Background(), "note-author", "a@example.com", "Author", models.RoleUser)
	if err != nil {
		t.Fatalf("seed author failed: %v", err)
	}
	user := asUser
	if user == nil {
		user = author
	}

	router := mux.NewRouter()
	router.Use(StubAuthMiddleware(user))
	handler := NewCommentsHandler(comments)
	router.HandleFunc("/api/notes/{noteId}/comments", handler.List).Methods(http.MethodGet)
	router.HandleFunc("/api/notes/{noteId}/comments", handler.Create).Methods(http.MethodPost)
	router.HandleFunc("/api/comments/{id}", handler.Update).Methods(http.MethodPut)
	router.HandleFunc("/api/comments/{id}", handler.Delete).Methods(http.MethodDelete)

	return router, notes, comments, author
}

func TestCommentsHandler_CreateThenList(t *testing.T) {
	router, notes, _, author := newCommentsTestServer(t, nil)
	note, err := notes.Create(context.Background(), "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("seed note failed: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"content": "Nice note!"})
	req := httptest.NewRequest(http.MethodPost, "/api/notes/"+itoa(note.ID)+"/comments", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/notes/"+itoa(note.ID)+"/comments", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
}

func TestCommentsHandler_DeleteByNonAuthorIsForbidden(t *testing.T) {
	router, notes, comments, author := newCommentsTestServer(t, nil)
	note, err := notes.Create(context.Background(), "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("seed note failed: %v", err)
	}
	comment, err := comments.Create(context.Background(), note.ID, "A comment", author.ID)
	if err != nil {
		t.Fatalf("seed comment failed: %v", err)
	}

	intruder := &models.User{ID: author.ID + 999, Role: models.RoleUser}
	router.Use(StubAuthMiddleware(intruder))

	req := httptest.NewRequest(http.MethodDelete, "/api/comments/"+itoa(comment.ID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCommentsHandler_CreateOnDeletedNote_Returns404(t *testing.T) {
	router, notes, _, author := newCommentsTestServer(t, nil)
	note, err := notes.Create(context.Background(), "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("seed note failed: %v", err)
	}
	if err := notes.SoftDelete(context.Background(), note.ID); err != nil {
		t.Fatalf("soft delete note failed: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"content": "Too late"})
	req := httptest.NewRequest(http.MethodPost, "/api/notes/"+itoa(note.ID)+"/comments", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
