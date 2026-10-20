package api

import (
	"encoding/json"
	"net/http"

	"notebook/internal/markdown"
)

type renderRequest struct {
	Content string `json:"content"`
}

func RenderPreviewHandler(w http.ResponseWriter, r *http.Request) {
	var req renderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	html, err := markdown.Render(req.Content)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to render markdown")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"html": html})
}
