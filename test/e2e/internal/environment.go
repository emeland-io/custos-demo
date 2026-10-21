package internal

import (
	"context"
	"fmt"
)

const (
	dexIssuer   = "http://dex.notebook-e2e.svc.cluster.local:5556"
	appBaseURL  = "http://127.0.0.1:18080"
	redirectURL = appBaseURL + "/auth/callback"
)

// Up brings the full e2e environment online: kind cluster, shared
// cluster dependencies, a self-signed ClusterIssuer, the test
// namespace, Dex, the app's secrets, the built image, the real Helm
// chart (which creates its own CNPG Cluster), and a port-forward to
// reach it. Steps run in strict order; each is individually idempotent
// (see Tasks 2-6), so re-running Up after a partial failure is safe.
func Up(ctx context.Context) error {
	if err := EnsureCluster(ctx, DefaultClusterLister, KindClusterName); err != nil {
		return fmt.Errorf("ensuring kind cluster: %w", err)
	}
	if err := InstallClusterDependencies(ctx); err != nil {
		return fmt.Errorf("installing cluster dependencies: %w", err)
	}
	if err := EnsureSelfSignedClusterIssuer(ctx); err != nil {
		return fmt.Errorf("ensuring self-signed ClusterIssuer: %w", err)
	}
	if err := EnsureTestNamespace(ctx); err != nil {
		return fmt.Errorf("ensuring test namespace: %w", err)
	}

	clientSecret, err := GenerateDexClientSecret()
	if err != nil {
		return fmt.Errorf("generating Dex client secret: %w", err)
	}
	dex := DexConfig{
		Issuer:       dexIssuer,
		ClientID:     "notebook",
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
	}
	if err := EnsureDex(ctx, dex); err != nil {
		return fmt.Errorf("ensuring Dex: %w", err)
	}

	hashKey, err := GenerateSessionKey()
	if err != nil {
		return fmt.Errorf("generating session hash key: %w", err)
	}
	blockKey, err := GenerateSessionKey()
	if err != nil {
		return fmt.Errorf("generating session block key: %w", err)
	}
	if err := EnsureAppSecrets(ctx, dex, []byte(hashKey), []byte(blockKey)); err != nil {
		return fmt.Errorf("ensuring app secrets: %w", err)
	}

	imageTag := "notebook:e2e"
	if err := BuildAndLoadImage(ctx, imageTag); err != nil {
		return fmt.Errorf("building/loading image: %w", err)
	}
	if err := InstallChart(ctx, imageTag, dex); err != nil {
		return fmt.Errorf("installing chart: %w", err)
	}
	// Defensive readiness gate on the chart's own CNPG Cluster: helm's
	// --wait only natively tracks Deployments/Jobs, not this CRD, and
	// the chart install above is what actually creates this Cluster.
	if err := WaitForClusterReady(ctx, TestNamespace, PostgresClusterName); err != nil {
		return fmt.Errorf("waiting for chart's Postgres Cluster: %w", err)
	}

	pid, err := StartPortForward(ctx)
	if err != nil {
		return fmt.Errorf("starting port-forward: %w", err)
	}

	return SaveState(State{BaseURL: appBaseURL, DexIssuer: dexIssuer, PortForwardPID: pid})
}

// Down tears down the test namespace (Dex, the chart release, the
// CNPG Cluster it created) in reverse order, stopping the port-forward
// first. It leaves cert-manager, the ingress controller, the
// Prometheus Operator, the CNPG operator, and the kind cluster itself
// running, unless deleteCluster is true.
func Down(ctx context.Context, deleteCluster bool) error {
	state, err := LoadState()
	if err == nil && state.PortForwardPID != 0 {
		_ = StopPortForward(state.PortForwardPID)
	}

	if err := runHelmUninstall(ctx, ReleaseName, TestNamespace); err != nil {
		return fmt.Errorf("uninstalling chart: %w", err)
	}
	if err := runKubectlDelete(ctx, "namespace", TestNamespace); err != nil {
		return fmt.Errorf("deleting test namespace: %w", err)
	}

	if deleteCluster {
		if err := DeleteCluster(ctx, KindClusterName); err != nil {
			return fmt.Errorf("deleting kind cluster: %w", err)
		}
	}

	return ClearState()
}
