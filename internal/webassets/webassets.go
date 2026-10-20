package webassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded frontend build output rooted at "dist", so
// callers use FS.Open("index.html") rather than FS.Open("dist/index.html").
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
