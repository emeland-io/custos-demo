package markdown

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

var sanitizer = bluemonday.UGCPolicy()

// Render converts Markdown source to sanitized HTML safe to send to
// the client as-is.
func Render(source string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(source), &buf); err != nil {
		return "", err
	}
	return sanitizer.Sanitize(buf.String()), nil
}
