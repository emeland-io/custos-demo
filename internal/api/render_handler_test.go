package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderPreviewHandler_RendersSanitizedHTML(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"content": "# Hi <script>alert(1)</script>"})
	req := httptest.NewRequest(http.MethodPost, "/api/render-preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	RenderPreviewHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !strings.HasPrefix(resp.HTML, "<h1>Hi") {
		t.Fatalf("expected rendered heading, got: %s", resp.HTML)
	}
	if strings.Contains(resp.HTML, "<script>") {
		t.Fatalf("expected script tag stripped, got: %s", resp.HTML)
	}
	if strings.Contains(resp.HTML, "raw HTML omitted") {
		t.Fatalf("expected no leaked raw-HTML-omitted markers, got: %s", resp.HTML)
	}
}
