package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	"notebook/test/e2e/internal"
)

// Task 9's own workflow runs `up` from the repo root but
// `baseurl`/`dexaddr` from frontend/ (still inside the module tree,
// just a different directory) — so the state file must live at a
// path anchored to the repo root, not wherever the calling process's
// cwd happens to be, or LoadState silently can't find what SaveState
// wrote.
func TestSaveAndLoadState_RoundTripsFromADifferentInTreeCWD(t *testing.T) {
	// SaveState/LoadState/ClearState always act on the one real state
	// file a live `devcluster up` also uses — this test must not
	// clobber or delete whatever (if anything) a real environment left
	// there, so it backs that up and restores it exactly afterward
	// instead of unconditionally clearing.
	root, err := internal.RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot failed: %v", err)
	}
	statePath := filepath.Join(root, ".devcluster-state.json")
	preexisting, readErr := os.ReadFile(statePath)
	t.Cleanup(func() {
		if readErr == nil {
			_ = os.WriteFile(statePath, preexisting, 0o600)
		} else {
			_ = os.Remove(statePath)
		}
	})

	original := internal.State{BaseURL: "http://127.0.0.1:18080", DexLocalAddr: "127.0.0.1:15556"}
	if err := internal.SaveState(original); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	frontendDir := filepath.Join(root, "frontend")
	if err := os.Chdir(frontendDir); err != nil {
		t.Fatalf("Chdir to %s failed: %v", frontendDir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	loaded, err := internal.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed from %s: %v", frontendDir, err)
	}
	if loaded.BaseURL != original.BaseURL || loaded.DexLocalAddr != original.DexLocalAddr {
		t.Fatalf("expected %+v, got %+v", original, loaded)
	}
}
