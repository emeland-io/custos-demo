package internal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type HelmRelease struct {
	Name      string
	Namespace string
	Chart     string
	Repo      string
	RepoName  string
	Version   string
	SetValues []string
}

type ReleaseLister interface {
	ListReleases(ctx context.Context, namespace string) ([]string, error)
}

type helmReleaseLister struct{}

func (helmReleaseLister) ListReleases(ctx context.Context, namespace string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "helm", "list", "-n", namespace, "-q").Output()
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

var DefaultReleaseLister ReleaseLister = helmReleaseLister{}

func alreadyInstalled(existing []string, name string) bool {
	for _, r := range existing {
		if r == name {
			return true
		}
	}
	return false
}

func EnsureHelmRelease(ctx context.Context, lister ReleaseLister, rel HelmRelease) error {
	existing, err := lister.ListReleases(ctx, rel.Namespace)
	if err != nil {
		return fmt.Errorf("listing helm releases in %s: %w", rel.Namespace, err)
	}
	if alreadyInstalled(existing, rel.Name) {
		return nil
	}

	if rel.Repo != "" {
		if err := exec.CommandContext(ctx, "helm", "repo", "add", rel.RepoName, rel.Repo).Run(); err != nil {
			return fmt.Errorf("adding helm repo %s: %w", rel.RepoName, err)
		}
		if err := exec.CommandContext(ctx, "helm", "repo", "update", rel.RepoName).Run(); err != nil {
			return fmt.Errorf("updating helm repo %s: %w", rel.RepoName, err)
		}
	}

	args := []string{"upgrade", "--install", rel.Name, rel.Chart, "-n", rel.Namespace, "--create-namespace", "--wait"}
	if rel.Version != "" {
		args = append(args, "--version", rel.Version)
	}
	for _, kv := range rel.SetValues {
		args = append(args, "--set", kv)
	}

	if err := exec.CommandContext(ctx, "helm", args...).Run(); err != nil {
		return fmt.Errorf("installing helm release %s: %w", rel.Name, err)
	}
	return nil
}

func ClusterDependencies() []HelmRelease {
	return []HelmRelease{
		{
			Name: "cert-manager", Namespace: "cert-manager",
			RepoName: "jetstack", Repo: "https://charts.jetstack.io",
			Chart: "jetstack/cert-manager", Version: "v1.16.1",
			SetValues: []string{"installCRDs=true"},
		},
		{
			Name: "ingress-nginx", Namespace: "ingress-nginx",
			RepoName: "ingress-nginx", Repo: "https://kubernetes.github.io/ingress-nginx",
			Chart: "ingress-nginx/ingress-nginx",
		},
		{
			Name: "kube-prometheus-stack", Namespace: "monitoring",
			RepoName: "prometheus-community", Repo: "https://prometheus-community.github.io/helm-charts",
			Chart:     "prometheus-community/kube-prometheus-stack",
			SetValues: []string{"grafana.enabled=false", "alertmanager.enabled=false"},
		},
		{
			Name: "cnpg", Namespace: "cnpg-system",
			RepoName: "cnpg", Repo: "https://cloudnative-pg.github.io/charts",
			Chart: "cnpg/cloudnative-pg", Version: "0.22.1",
		},
	}
}

func InstallClusterDependencies(ctx context.Context) error {
	for _, dep := range ClusterDependencies() {
		if err := EnsureHelmRelease(ctx, DefaultReleaseLister, dep); err != nil {
			return fmt.Errorf("installing %s: %w", dep.Name, err)
		}
	}
	return nil
}
