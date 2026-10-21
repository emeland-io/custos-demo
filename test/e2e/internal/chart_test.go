package internal_test

import (
	"testing"

	"notebook/test/e2e/internal"
)

func TestSplitImageTag(t *testing.T) {
	cases := []struct {
		in, repo, tag string
	}{
		{"ghcr.io/example/notebook:abc123", "ghcr.io/example/notebook", "abc123"},
		{"notebook:dev", "notebook", "dev"},
		{"notebook", "notebook", "latest"},
	}
	for _, c := range cases {
		repo, tag := internal.SplitImageTag(c.in)
		if repo != c.repo || tag != c.tag {
			t.Fatalf("SplitImageTag(%q) = (%q, %q), want (%q, %q)", c.in, repo, tag, c.repo, c.tag)
		}
	}
}
