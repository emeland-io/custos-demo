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
