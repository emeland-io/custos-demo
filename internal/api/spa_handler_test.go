package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":     {Data: []byte("<html>spa shell</html>")},
		"assets/app.js":  {Data: []byte("console.log('app')")},
		"assets/app.css": {Data: []byte("body{color:red}")},
	}
}

func TestSPAHandler_ServesExistingAsset(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "console.log('app')" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestSPAHandler_FallsBackToIndexForUnknownNavigationalPath(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/notes/42", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>spa shell</html>" {
		t.Fatalf("expected index.html fallback, got: %s", rec.Body.String())
	}
}

func TestSPAHandler_ServesIndexAtRoot(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "<html>spa shell</html>" {
		t.Fatalf("expected index.html at root, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_MissingAssetLikePath_404sInsteadOfFallback(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing asset path, got %d: %s", rec.Code, rec.Body.String())
	}
}
