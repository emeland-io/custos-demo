package internal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// WaitForTCPPort blocks until addr accepts a TCP connection or
// timeout elapses.
func WaitForTCPPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s to accept connections", addr)
}

// RepoRoot returns the directory containing this module's go.mod.
// BuildAndLoadImage and InstallChart need it as an explicit exec.Cmd
// Dir: a Ginkgo suite invoked via `go test ./test/e2e/...` runs with
// its working directory set to the test/e2e package directory, not
// the repo root, so relative paths like "." (docker build context) or
// "deploy/helm/notebook" (the chart) cannot rely on the caller's cwd.
func RepoRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("determining repo root via go env GOMOD: %w", err)
	}
	return filepath.Dir(strings.TrimSpace(string(out))), nil
}

// ReleaseName is the Helm release name used for the app chart in the
// e2e environment. It feeds notebook.fullname (ReleaseName + "-notebook")
// in deploy/helm/notebook/templates/_helpers.tpl, which in turn names
// the chart's own CNPG Cluster (<fullname>-postgres) and, via CNPG's
// auto-generated app-user Secret naming convention, its credentials
// Secret (<fullname>-postgres-app).
const ReleaseName = "notebook-e2e"

// PostgresAppSecretName is the CNPG-generated credentials Secret for
// the Cluster the chart itself creates (see ReleaseName's comment).
// This is NOT a separately-managed test Cluster — Plan 5's chart
// unconditionally creates its own Cluster, so this plan must point at
// that Cluster's generated Secret rather than standing up a second,
// unused one.
const PostgresAppSecretName = ReleaseName + "-notebook-postgres-app"

// PostgresClusterName is the chart's own CNPG Cluster name, used to
// wait for it to become Ready after installing the chart.
const PostgresClusterName = ReleaseName + "-notebook-postgres"

func SplitImageTag(full string) (repo, tag string) {
	idx := strings.LastIndexByte(full, ':')
	if idx < 0 {
		return full, "latest"
	}
	return full[:idx], full[idx+1:]
}

func BuildAndLoadImage(ctx context.Context, tag string) error {
	root, err := RepoRoot()
	if err != nil {
		return err
	}
	build := exec.CommandContext(ctx, "docker", "build", "-t", tag, ".")
	build.Dir = root
	if err := build.Run(); err != nil {
		return fmt.Errorf("building image %s: %w", tag, err)
	}
	if err := exec.CommandContext(ctx, "kind", "load", "docker-image", tag, "--name", KindClusterName).Run(); err != nil {
		return fmt.Errorf("loading image %s into kind: %w", tag, err)
	}
	return nil
}

func randomHex(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateDexClientSecret and GenerateSessionKeys produce fresh random
// values; nothing here is ever committed. Callers only use these when
// ChooseSecretValue determines nothing already exists — see its doc
// comment for why regenerating unconditionally on every `up` breaks
// re-runs without a `down` in between.
func GenerateDexClientSecret() (string, error) { return randomHex(16) }
func GenerateSessionKey() (string, error)      { return randomHex(16) }

// ChooseSecretValue reuses an already-existing secret value instead
// of generating a new one. Dex restarts on every `up` to pick up its
// rendered config (see the checksum/config annotation in dex.go), but
// the app Deployment never restarts just because a Secret it mounts
// via secretKeyRef changed its value — env vars from secretKeyRef are
// captured once, at pod start. Regenerating the Dex client secret (or
// the session hash/block keys) on every `up` therefore desyncs Dex's
// freshly-rotated secret from the app's still-stale one, and Dex
// starts rejecting the app's token requests outright. Reusing
// whatever is already there keeps repeated `up` runs idempotent, as
// the plan's Review Focus requires.
func ChooseSecretValue(existing string, exists bool, generate func() (string, error)) (string, error) {
	if exists {
		return existing, nil
	}
	return generate()
}

// readSecretKey reads one base64-decoded data key from an existing
// Secret, reporting (via exists) whether the Secret (and that key)
// are present at all rather than treating "not found" as an error —
// that's the expected, idempotent case on a genuinely fresh `up`.
func readSecretKey(ctx context.Context, secretName, key string) (value string, exists bool, err error) {
	out, err := exec.CommandContext(ctx, "kubectl", "--context", KindContextName, "get", "secret", secretName,
		"-n", TestNamespace, "-o", fmt.Sprintf("jsonpath={.data.%s}", key)).Output()
	if err != nil || len(out) == 0 {
		return "", false, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(string(out))
	if err != nil {
		return "", false, fmt.Errorf("decoding existing secret %s/%s: %w", secretName, key, err)
	}
	return string(decoded), true, nil
}

// getOrGenerateSecret combines readSecretKey with ChooseSecretValue:
// reuse the existing value in secretName's key if the Secret is
// already there, otherwise call generate (GenerateDexClientSecret or
// GenerateSessionKey).
func getOrGenerateSecret(ctx context.Context, secretName, key string, generate func() (string, error)) (string, error) {
	existing, exists, err := readSecretKey(ctx, secretName, key)
	if err != nil {
		return "", err
	}
	return ChooseSecretValue(existing, exists, generate)
}

func EnsureAppSecrets(ctx context.Context, dex DexConfig, sessionHashKey, sessionBlockKey []byte) error {
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: notebook-e2e-oidc
  namespace: %s
type: Opaque
data:
  client-id: %s
  client-secret: %s
---
apiVersion: v1
kind: Secret
metadata:
  name: notebook-e2e-session
  namespace: %s
type: Opaque
data:
  hash-key: %s
  block-key: %s
`,
		TestNamespace, base64.StdEncoding.EncodeToString([]byte(dex.ClientID)), base64.StdEncoding.EncodeToString([]byte(dex.ClientSecret)),
		TestNamespace, base64.StdEncoding.EncodeToString(sessionHashKey), base64.StdEncoding.EncodeToString(sessionBlockKey),
	)
	return ApplyManifest(ctx, manifest)
}

func InstallChart(ctx context.Context, imageTag string, dex DexConfig) error {
	repo, tag := SplitImageTag(imageTag)
	args := []string{
		"--kube-context", KindContextName,
		"upgrade", "--install", ReleaseName, "deploy/helm/notebook",
		"-n", TestNamespace, "--create-namespace", "--wait", "--timeout", "8m",
		"--set", "image.repository=" + repo,
		"--set", "image.tag=" + tag,
		"--set", "ingress.className=nginx",
		"--set", "ingress.host=notebook.notebook-e2e.svc.cluster.local",
		"--set", "ingress.tls.clusterIssuer=selfsigned-e2e",
		"--set", "postgresql.credentialsSecretRef.name=" + PostgresAppSecretName,
		"--set", "oidc.issuerURL=" + dex.Issuer,
		"--set", "oidc.redirectURL=" + dex.RedirectURL,
		"--set", "oidc.adminClaim=email",
		"--set", "oidc.adminClaimValue=" + TestAdminEmail,
		"--set", "oidc.clientIDSecretRef.name=notebook-e2e-oidc",
		"--set", "oidc.clientSecretSecretRef.name=notebook-e2e-oidc",
		"--set", "session.hashKeySecretRef.name=notebook-e2e-session",
		"--set", "session.blockKeySecretRef.name=notebook-e2e-session",
	}
	root, err := RepoRoot()
	if err != nil {
		return err
	}
	install := exec.CommandContext(ctx, "helm", args...)
	install.Dir = root
	if err := install.Run(); err != nil {
		return fmt.Errorf("installing notebook chart: %w", err)
	}
	return nil
}

func startPortForward(service, portMapping, localAddr string) (int, error) {
	cmd := exec.Command("kubectl", "--context", KindContextName, "port-forward", "-n", TestNamespace, service, portMapping)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting port-forward to %s: %w", service, err)
	}
	// cmd.Start returning only means the kubectl process has been
	// forked; the tunnel itself takes a moment longer to actually
	// accept connections, and a caller that proceeds immediately can
	// race it and see "connection refused" on its first request.
	if err := WaitForTCPPort(localAddr, 30*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return 0, fmt.Errorf("port-forward to %s never became reachable: %w", service, err)
	}
	return cmd.Process.Pid, nil
}

func StartPortForward(ctx context.Context) (int, error) {
	return startPortForward("svc/notebook-e2e-notebook", "18080:8080", "127.0.0.1:18080")
}

// StartDexPortForward forwards Dex's Service to a fixed local port,
// so a client running outside the cluster (NewCookieClient's caller,
// or Playwright's browser) can reach Dex's authorization endpoint —
// whose URL otherwise names a hostname that only resolves inside the
// cluster's own DNS.
func StartDexPortForward(ctx context.Context) (pid int, localAddr string, err error) {
	const addr = "127.0.0.1:15556"
	pid, err = startPortForward("svc/dex", "15556:5556", addr)
	return pid, addr, err
}

func StopPortForward(pid int) error {
	if pid == 0 {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
