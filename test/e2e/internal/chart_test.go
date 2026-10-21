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

// Up() must be idempotent when re-run without a Down in between (the
// plan's own Review Focus calls this out explicitly). Dex's config
// and the app's OIDC/session Secrets both embed randomly generated
// values (client secret, hash/block keys); regenerating fresh values
// on every run desyncs Dex (which restarts to pick up its new config,
// via the checksum/config annotation) from the app Deployment (which
// never restarts just because a Secret it mounts via secretKeyRef
// changed), causing Dex to reject the app's now-stale client secret
// ("invalid client_secret on token request"). chooseSecretValue is
// the pure decision — reuse what's already there, generate only when
// nothing exists yet — that EnsureAppSecrets' live kubectl lookups
// build on.
func TestChooseSecretValue_ReusesExistingWhenPresent(t *testing.T) {
	calls := 0
	generate := func() (string, error) { calls++; return "freshly-generated", nil }

	got, err := internal.ChooseSecretValue("already-there", true, generate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "already-there" {
		t.Fatalf("expected to reuse the existing value, got %q", got)
	}
	if calls != 0 {
		t.Fatalf("expected generate not to be called when a value already exists, called %d times", calls)
	}
}

func TestChooseSecretValue_GeneratesWhenAbsent(t *testing.T) {
	generate := func() (string, error) { return "freshly-generated", nil }

	got, err := internal.ChooseSecretValue("", false, generate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "freshly-generated" {
		t.Fatalf("expected the generated value, got %q", got)
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
