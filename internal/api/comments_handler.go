package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"notebook/internal/metrics"
	"notebook/internal/repo"
)

type CommentsHandler struct {
	comments *repo.CommentRepo
}

func NewCommentsHandler(comments *repo.CommentRepo) *CommentsHandler {
	return &CommentsHandler{comments: comments}
}

type commentRequest struct {
	Content string `json:"content"`
}

type commentResponse struct {
	ID        int64  `json:"id"`
	NoteID    int64  `json:"note_id"`
	Content   string `json:"content"`
	AuthorID  int64  `json:"author_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (h *CommentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	noteID, err := strconv.ParseInt(mux.Vars(r)["noteId"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	var req commentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	c, err := h.comments.Create(r.Context(), noteID, req.Content, user.ID)
	if errors.Is(err, repo.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "note not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create comment")
		return
	}
	metrics.CommentsCreatedTotal.Inc()

	writeJSON(w, http.StatusCreated, commentResponse{
		ID: c.ID, NoteID: c.NoteID, Content: c.Content, AuthorID: c.AuthorID,
		CreatedAt: c.CreatedAt.Format(timeLayout), UpdatedAt: c.UpdatedAt.Format(timeLayout),
	})
}

func (h *CommentsHandler) List(w http.ResponseWriter, r *http.Request) {
	noteID, err := strconv.ParseInt(mux.Vars(r)["noteId"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	list, err := h.comments.ListByNoteID(r.Context(), noteID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	items := make([]commentResponse, 0, len(list))
	for _, c := range list {
		items = append(items, commentResponse{
			ID: c.ID, NoteID: c.NoteID, Content: c.Content, AuthorID: c.AuthorID,
			CreatedAt: c.CreatedAt.Format(timeLayout), UpdatedAt: c.UpdatedAt.Format(timeLayout),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *CommentsHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid comment id")
		return
	}

	existing, err := h.comments.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "comment not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may edit this comment")
		return
	}

	var req commentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	updated, err := h.comments.Update(r.Context(), id, req.Content)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update comment")
		return
	}

	writeJSON(w, http.StatusOK, commentResponse{
		ID: updated.ID, NoteID: updated.NoteID, Content: updated.Content, AuthorID: updated.AuthorID,
		CreatedAt: updated.CreatedAt.Format(timeLayout), UpdatedAt: updated.UpdatedAt.Format(timeLayout),
	})
}

func (h *CommentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid comment id")
		return
	}

	existing, err := h.comments.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "comment not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may delete this comment")
		return
	}

	if err := h.comments.SoftDelete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete comment")
		return
	}
	metrics.CommentsDeletedTotal.Inc()
	w.WriteHeader(http.StatusNoContent)
}
