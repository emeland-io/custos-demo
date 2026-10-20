package markdown_test

import (
	"strings"
	"testing"

	"notebook/internal/markdown"
)

func TestRender_BasicMarkdown(t *testing.T) {
	html, err := markdown.Render("# Hello\n\nThis is **bold**.")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(html, "<h1>Hello</h1>") {
		t.Fatalf("expected h1 heading, got: %s", html)
	}
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Fatalf("expected strong tag, got: %s", html)
	}
}

func TestRender_StripsScriptTag(t *testing.T) {
	html, err := markdown.Render("hello <script>alert('xss')</script> world")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("expected script tag to be stripped, got: %s", html)
	}
}

func TestRender_StripsOnErrorAttribute(t *testing.T) {
	html, err := markdown.Render(`<img src="x" onerror="alert(1)">`)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if strings.Contains(html, "onerror") {
		t.Fatalf("expected onerror attribute to be stripped, got: %s", html)
	}
}
