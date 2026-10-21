package internal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const KindClusterName = "notebook-e2e"

// KindContextName is the kubeconfig context kind names its cluster
// ("kind-<cluster-name>"). Every kubectl/helm call below must pin
// this explicitly: otherwise up/down act on whatever context the
// caller's kubeconfig *currently* points at, which — unlike kind
// itself switching context on first `kind create` — is never
// re-selected on a later run that skips creation because the cluster
// already exists. A developer who has since switched kubeconfig
// context to a shared or production cluster and reruns `devcluster
// up` would otherwise install cert-manager, ingress-nginx,
// kube-prometheus-stack, CNPG and the app into THAT cluster.
const KindContextName = "kind-" + KindClusterName

type ClusterLister interface {
	ListClusters(ctx context.Context) ([]string, error)
}

type kindClusterLister struct{}

func (kindClusterLister) ListClusters(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "kind", "get", "clusters").Output()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func clusterExists(existing []string, name string) bool {
	for _, n := range existing {
		if n == name {
			return true
		}
	}
	return false
}

func EnsureCluster(ctx context.Context, lister ClusterLister, name string) error {
	existing, err := lister.ListClusters(ctx)
	if err != nil {
		return fmt.Errorf("listing kind clusters: %w", err)
	}
	if clusterExists(existing, name) {
		return nil
	}
	if err := exec.CommandContext(ctx, "kind", "create", "cluster", "--name", name).Run(); err != nil {
		return fmt.Errorf("creating kind cluster %s: %w", name, err)
	}
	return nil
}

func DeleteCluster(ctx context.Context, name string) error {
	if err := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", name).Run(); err != nil {
		return fmt.Errorf("deleting kind cluster %s: %w", name, err)
	}
	return nil
}

// DefaultClusterLister is the real, kind-backed ClusterLister used
// outside of tests.
var DefaultClusterLister ClusterLister = kindClusterLister{}
