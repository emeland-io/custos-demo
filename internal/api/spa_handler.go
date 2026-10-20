package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// NewSPAHandler serves files from assets by exact path. A request
// whose path doesn't exist in assets and looks like a navigational
// route (no file extension) falls back to index.html, so vue-router's
// HTML5 history mode works without a separate web server. A request
// for a path with a file extension (e.g. "/assets/missing.js") that
// doesn't exist 404s instead, so broken asset references are visible
// as 404s in the browser's network tab rather than masquerading as
// index.html and failing with a confusing script-parse error.
func NewSPAHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleaned := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleaned == "" || cleaned == "." {
			cleaned = "index.html"
		}

		if _, err := fs.Stat(assets, cleaned); err != nil {
			if path.Ext(cleaned) != "" {
				http.NotFound(w, r)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}
