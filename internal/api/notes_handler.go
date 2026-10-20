package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"notebook/internal/metrics"
	"notebook/internal/repo"
)

type NotesHandler struct {
	notes *repo.NoteRepo
}

func NewNotesHandler(notes *repo.NoteRepo) *NotesHandler {
	return &NotesHandler{notes: notes}
}

type noteRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type noteResponse struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	AuthorID  int64  `json:"author_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *NotesHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "title and content are required")
		return
	}

	note, err := h.notes.Create(r.Context(), req.Title, req.Content, user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create note")
		return
	}
	metrics.NotesCreatedTotal.Inc()

	writeJSON(w, http.StatusCreated, noteResponse{
		ID: note.ID, Title: note.Title, Content: note.Content, AuthorID: note.AuthorID,
		CreatedAt: note.CreatedAt.Format(timeLayout), UpdatedAt: note.UpdatedAt.Format(timeLayout),
	})
}

func (h *NotesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	note, err := h.notes.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "note not found")
		return
	}

	writeJSON(w, http.StatusOK, noteResponse{
		ID: note.ID, Title: note.Title, Content: note.Content, AuthorID: note.AuthorID,
		CreatedAt: note.CreatedAt.Format(timeLayout), UpdatedAt: note.UpdatedAt.Format(timeLayout),
	})
}

func (h *NotesHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	existing, err := h.notes.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "note not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may edit this note")
		return
	}

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "title and content are required")
		return
	}

	updated, err := h.notes.Update(r.Context(), id, req.Title, req.Content)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update note")
		return
	}

	writeJSON(w, http.StatusOK, noteResponse{
		ID: updated.ID, Title: updated.Title, Content: updated.Content, AuthorID: updated.AuthorID,
		CreatedAt: updated.CreatedAt.Format(timeLayout), UpdatedAt: updated.UpdatedAt.Format(timeLayout),
	})
}

func (h *NotesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	existing, err := h.notes.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "note not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may delete this note")
		return
	}

	if err := h.notes.SoftDelete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete note")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *NotesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := repo.NoteListParams{
		Search: strings.TrimSpace(q.Get("search")),
		Limit:  50,
	}
	if authorStr := q.Get("author_id"); authorStr != "" {
		authorID, err := strconv.ParseInt(authorStr, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid author_id")
			return
		}
		params.AuthorID = &authorID
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid offset")
			return
		}
		params.Offset = offset
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		params.Limit = limit
	}

	notes, total, err := h.notes.List(r.Context(), params)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list notes")
		return
	}

	items := make([]noteResponse, 0, len(notes))
	for _, n := range notes {
		items = append(items, noteResponse{
			ID: n.ID, Title: n.Title, Content: n.Content, AuthorID: n.AuthorID,
			CreatedAt: n.CreatedAt.Format(timeLayout), UpdatedAt: n.UpdatedAt.Format(timeLayout),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
