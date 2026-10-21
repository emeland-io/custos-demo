package internal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

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
	if err := exec.CommandContext(ctx, "docker", "build", "-t", tag, ".").Run(); err != nil {
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
// values every `up`; nothing here is ever committed.
func GenerateDexClientSecret() (string, error) { return randomHex(16) }
func GenerateSessionKey() (string, error)      { return randomHex(16) }

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
	if err := exec.CommandContext(ctx, "helm", args...).Run(); err != nil {
		return fmt.Errorf("installing notebook chart: %w", err)
	}
	return nil
}

func StartPortForward(ctx context.Context) (int, error) {
	cmd := exec.Command("kubectl", "port-forward", "-n", TestNamespace, "svc/notebook-e2e-notebook", "18080:8080")
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting port-forward: %w", err)
	}
	return cmd.Process.Pid, nil
}

func StopPortForward(pid int) error {
	if pid == 0 {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
