package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/metrics"
	"notebook/internal/repo"
)

func NewRouter(logger *slog.Logger, notes *repo.NoteRepo, comments *repo.CommentRepo) http.Handler {
	r := mux.NewRouter()
	r.Use(metrics.Middleware)
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)
	r.Handle("/metrics", metrics.Handler()).Methods(http.MethodGet)
	r.HandleFunc("/api/render-preview", RenderPreviewHandler).Methods(http.MethodPost)

	notesHandler := NewNotesHandler(notes)
	r.HandleFunc("/api/notes", notesHandler.List).Methods(http.MethodGet)
	r.HandleFunc("/api/notes", notesHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/api/notes/{id}", notesHandler.Get).Methods(http.MethodGet)
	r.HandleFunc("/api/notes/{id}", notesHandler.Update).Methods(http.MethodPut)
	r.HandleFunc("/api/notes/{id}", notesHandler.Delete).Methods(http.MethodDelete)

	commentsHandler := NewCommentsHandler(comments)
	r.HandleFunc("/api/notes/{noteId}/comments", commentsHandler.List).Methods(http.MethodGet)
	r.HandleFunc("/api/notes/{noteId}/comments", commentsHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/api/comments/{id}", commentsHandler.Update).Methods(http.MethodPut)
	r.HandleFunc("/api/comments/{id}", commentsHandler.Delete).Methods(http.MethodDelete)

	return r
}
