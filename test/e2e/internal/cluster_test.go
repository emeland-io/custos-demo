package internal_test

import (
	"context"
	"testing"

	"notebook/test/e2e/internal"
)

type fakeClusterLister struct{ clusters []string }

func (f fakeClusterLister) ListClusters(ctx context.Context) ([]string, error) {
	return f.clusters, nil
}

func TestEnsureCluster_SkipsCreationWhenAlreadyPresent(t *testing.T) {
	lister := fakeClusterLister{clusters: []string{internal.KindClusterName}}
	if err := internal.EnsureCluster(context.Background(), lister, internal.KindClusterName); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
