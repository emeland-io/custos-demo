package internal_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"notebook/test/e2e/internal"
)

// BuildAndLoadImage/InstallChart shell out to `docker build .` and
// `helm ... deploy/helm/notebook`, both repo-root-relative paths. A
// Ginkgo suite run via `go test ./test/e2e/...` executes with its
// working directory set to the test/e2e package directory, not the
// repo root, so these commands must run with an explicit repo-root
// Dir rather than relying on the caller's cwd.
func TestRepoRoot_ContainsGoMod(t *testing.T) {
	root, err := internal.RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected %s/go.mod to exist: %v", root, err)
	}
	if _, err := os.Stat(filepath.Join(root, "deploy", "helm", "notebook", "Chart.yaml")); err != nil {
		t.Fatalf("expected %s/deploy/helm/notebook/Chart.yaml to exist: %v", root, err)
	}
}

// `kubectl port-forward`'s process starting (cmd.Start returning) is
// not the same event as the tunnel actually accepting connections —
// StartPortForward must not hand back control to its caller until the
// port is genuinely dialable, or the caller's first request races the
// tunnel and gets "connection refused".
func TestWaitForTCPPort_ReturnsOnceSomethingListens(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open a test listener: %v", err)
	}
	defer ln.Close()

	go func() {
		time.Sleep(200 * time.Millisecond)
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	if err := internal.WaitForTCPPort(ln.Addr().String(), 2*time.Second); err != nil {
		t.Fatalf("expected WaitForTCPPort to succeed once something is listening, got %v", err)
	}
}

func TestWaitForTCPPort_TimesOutWhenNothingListens(t *testing.T) {
	// Port 0 is never a real listening address, so dialing it always
	// fails fast — this proves the function returns an error rather
	// than hanging forever when nothing ever comes up.
	if err := internal.WaitForTCPPort("127.0.0.1:0", 300*time.Millisecond); err == nil {
		t.Fatalf("expected a timeout error when nothing listens, got nil")
	}
}

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
