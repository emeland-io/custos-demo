package api

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/metrics"
	"notebook/internal/repo"
)

type AuthWiring struct {
	SessionMiddleware func(http.Handler) http.Handler
	LoginHandler      http.HandlerFunc
	CallbackHandler   http.HandlerFunc
	LogoutHandler     http.HandlerFunc
}

func NewRouter(logger *slog.Logger, notes *repo.NoteRepo, comments *repo.CommentRepo, auth AuthWiring, assets fs.FS) http.Handler {
	r := mux.NewRouter()
	r.Use(metrics.Middleware)
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)
	r.Handle("/metrics", metrics.Handler()).Methods(http.MethodGet)

	r.HandleFunc("/auth/login", auth.LoginHandler).Methods(http.MethodGet)
	r.HandleFunc("/auth/callback", auth.CallbackHandler).Methods(http.MethodGet)
	r.HandleFunc("/auth/logout", auth.LogoutHandler).Methods(http.MethodPost)

	api := r.PathPrefix("/api").Subrouter()
	api.Use(auth.SessionMiddleware)
	api.HandleFunc("/me", MeHandler).Methods(http.MethodGet)
	api.HandleFunc("/render-preview", RenderPreviewHandler).Methods(http.MethodPost)

	notesHandler := NewNotesHandler(notes)
	api.HandleFunc("/notes", notesHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/notes", notesHandler.Create).Methods(http.MethodPost)
	api.HandleFunc("/notes/{id}", notesHandler.Get).Methods(http.MethodGet)
	api.HandleFunc("/notes/{id}", notesHandler.Update).Methods(http.MethodPut)
	api.HandleFunc("/notes/{id}", notesHandler.Delete).Methods(http.MethodDelete)

	commentsHandler := NewCommentsHandler(comments)
	api.HandleFunc("/notes/{noteId}/comments", commentsHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/notes/{noteId}/comments", commentsHandler.Create).Methods(http.MethodPost)
	api.HandleFunc("/comments/{id}", commentsHandler.Update).Methods(http.MethodPut)
	api.HandleFunc("/comments/{id}", commentsHandler.Delete).Methods(http.MethodDelete)

	// Catch-all for the SPA — registered last so it never shadows the
	// routes above; mux matches routes in registration order.
	r.PathPrefix("/").Handler(NewSPAHandler(assets))

	return r
}
