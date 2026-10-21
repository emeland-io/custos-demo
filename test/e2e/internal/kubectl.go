package internal

import (
	"context"
	"fmt"
	"os/exec"
)

func RolloutWait(ctx context.Context, namespace, resource string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "rollout", "status", resource, "-n", namespace, "--timeout=3m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("waiting for %s rollout in %s: %w", resource, namespace, err)
	}
	return nil
}

// WaitForClusterReady waits for a CNPG Cluster to report
// condition=Ready. helm's own --wait only natively tracks
// Deployments/Jobs/etc, not arbitrary CRDs like CNPG's Cluster, so
// this is a defensive readiness gate run after the chart install that
// creates the Cluster.
func WaitForClusterReady(ctx context.Context, namespace, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "wait", "--for=condition=Ready",
		"cluster/"+name, "-n", namespace, "--timeout=5m")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("waiting for cluster/%s to become ready in %s: %w", name, namespace, err)
	}
	return nil
}
