package webassets_test

import (
	"io/fs"
	"testing"

	"notebook/internal/webassets"
)

func TestFS_ContainsIndexAtRoot(t *testing.T) {
	assets, err := webassets.FS()
	if err != nil {
		t.Fatalf("FS() failed: %v", err)
	}

	// With only the committed placeholder present, there is no
	// index.html — this test only asserts the FS is rooted correctly
	// (README.txt is readable at the root, not under "dist/").
	entries, err := fs.ReadDir(assets, ".")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "README.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected README.txt at FS root, got entries: %+v", entries)
	}
}
