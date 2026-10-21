package internal_test

import (
	"context"
	"testing"

	"notebook/test/e2e/internal"
)

type fakeReleaseLister struct{ releases map[string][]string }

func (f fakeReleaseLister) ListReleases(ctx context.Context, namespace string) ([]string, error) {
	return f.releases[namespace], nil
}

func TestEnsureHelmRelease_SkipsInstallWhenAlreadyPresent(t *testing.T) {
	lister := fakeReleaseLister{releases: map[string][]string{"cert-manager": {"cert-manager"}}}
	rel := internal.HelmRelease{Name: "cert-manager", Namespace: "cert-manager", Chart: "jetstack/cert-manager"}

	if err := internal.EnsureHelmRelease(context.Background(), lister, rel); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestClusterDependencies_IncludesExpectedReleases(t *testing.T) {
	deps := internal.ClusterDependencies()
	names := map[string]bool{}
	for _, d := range deps {
		names[d.Name] = true
	}
	for _, want := range []string{"cert-manager", "ingress-nginx", "kube-prometheus-stack", "cnpg"} {
		if !names[want] {
			t.Fatalf("expected cluster dependencies to include %q, got %+v", want, deps)
		}
	}
}
