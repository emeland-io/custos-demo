package internal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const TestNamespace = "notebook-e2e"

func ApplyManifest(ctx context.Context, manifest string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--context", KindContextName, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("applying manifest: %w", err)
	}
	return nil
}

func EnsureTestNamespace(ctx context.Context) error {
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, TestNamespace)
	return ApplyManifest(ctx, manifest)
}

const selfSignedClusterIssuerManifest = `
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-e2e
spec:
  selfSigned: {}
`

func RenderSelfSignedClusterIssuerManifest() string {
	return selfSignedClusterIssuerManifest
}

func EnsureSelfSignedClusterIssuer(ctx context.Context) error {
	return ApplyManifest(ctx, RenderSelfSignedClusterIssuerManifest())
}
