# Notebook App — E2E Test Framework & CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a `devcluster`-style Go CLI that stands up a throwaway
**kind** cluster with cert-manager, an ingress controller, the
Prometheus Operator, the CloudNativePG operator, a **Dex** test OIDC
provider, and the real production Helm chart, assert against it with
**Ginkgo/Gomega**, add a **Playwright** suite that drives a real browser
through the full login-and-notes journey, and wire both into GitHub
Actions on every PR alongside the existing Go/frontend unit tests and
`helm lint`/`helm template`.

**Architecture:** `test/e2e/internal` holds small, mostly-`exec`-backed
orchestration functions (install a Helm release, apply a manifest, wait
for readiness); each function's *decision logic* (should I install
this? does this manifest render correctly?) is extracted into a pure,
unit-testable form, while the actual cluster-mutating calls are
verified by running the CLI against a real kind cluster — this mirrors
how this exact category of code is tested in the reference project
(timadorus/platform): by running it, not mocking `kind`/`helm`/
`kubectl`. `test/e2e/cmd/devcluster` is the CLI entry point with `up`,
`down`, and `baseurl` subcommands, persisting a small state file so
separate CI steps (bring the environment up, run Ginkgo, run
Playwright, tear it down) can share connection details across
process boundaries.

**Tech Stack:** Go (`os/exec` orchestration), `kind`, `helm`, `kubectl`,
Ginkgo v2 / Gomega behind a `//go:build e2e` tag, Dex (test OIDC
provider), Playwright (`@playwright/test`), GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-19-notebook-app.md`

## Handoff Note (read before starting)

This plan assumes Plan 5
(`docs/superpowers/plans/2026-10-19-notebook-helm-chart.md`) was
implemented exactly as written: the chart at `deploy/helm/notebook`
with the exact required value names in its README's "Installing"
example, the `--migrate-only` flag on `cmd/server/main.go`, and that
CloudNativePG auto-generates an app-user credentials Secret named
`<cluster-name>-app` with a `uri` key. It also assumes Plan 2's
`internal/config.OIDCConfig.AdminClaim`/`AdminClaimValue` accept any
claim name (not just `groups`), which this plan exploits by mapping
admin-ness to the `email` claim for the Dex test provider rather than a
groups claim (see Task 5's note on why). **Before this plan's first
task starts, the reviewer who signed off on Plan 5's final
whole-branch review must re-read this plan against the actual Plan
1–5 codebase** (chart values, CNPG secret naming, config field names)
and correct any drift before proceeding.

## Global Constraints

- Every `exec.Command`-based orchestration function separates its
  *decision* (should this action run at all?) from its *execution* (the
  actual shell-out), so the decision can be unit tested with a fake
  dependency and the execution is verified by actually running the CLI
  — never by mocking `kind`/`helm`/`kubectl` themselves.
- Teardown always runs in the exact reverse order of setup, and leaves
  cluster-wide shared installs (cert-manager, ingress controller,
  Prometheus Operator, CNPG operator, and the kind cluster itself)
  running by default — only the test namespace (Dex, the app's Helm
  release, the test Postgres `Cluster`) is torn down — so repeated
  local `up`/`down` cycles during development stay fast. A
  `--delete-cluster` flag on `down` removes the kind cluster entirely,
  for CI's final cleanup.
- The Dex test provider's credentials (client secret, session keys) are
  generated at `up` time, never committed, and only ever live inside
  the throwaway cluster's Secrets and the local state file.
- Both the Ginkgo suite and the Playwright suite drive the app through
  its real public HTTP surface (the Ingress, or a `kubectl
  port-forward` to the Service) — neither test suite talks to the
  database or calls internal Go packages directly.
- CI runs `backend-test`, `frontend-test`, and `helm-lint` on every PR
  regardless of outcome elsewhere; `e2e-cluster` (which also runs the
  Playwright suite against the same live environment) runs after those
  three succeed, since there's no point spinning up a kind cluster for
  code that doesn't even compile or lint.

## Review Focus

- `devcluster up` run twice in a row without a `down` in between (a
  developer re-running it after a partial failure) — every install step
  must be idempotent (skip, not error, when already present), matching
  the explicit skip-logic tests in Task 3.
- The Ginkgo suite's login-and-create-note test — must fail loudly (not
  hang until the 30-minute suite timeout) if Dex's login form markup
  doesn't match what `LoginViaDex` expects, since that's exactly the
  kind of drift a Dex version bump could introduce.
- `devcluster down` run when `up` never completed successfully (e.g. it
  failed on the CNPG operator install step) — must not panic on a
  missing/partial state file; it should tear down whatever did get
  created and exit cleanly.
- The Playwright suite's `beforeEach` login flow — must fail clearly if
  `E2E_BASE_URL` isn't set, rather than making a request to
  `undefined/notes` and producing a confusing network error.
- CI's `helm-lint` job — must actually exercise `postgresql.backup.enabled`
  at least once (with the full set of backup values) so the Plan 5
  `required` guards on that block don't silently bit-rot unexercised.

---

## File Structure

```
test/e2e/cmd/devcluster/main.go
test/e2e/internal/prereqs.go
test/e2e/internal/prereqs_test.go
test/e2e/internal/cluster.go
test/e2e/internal/cluster_test.go
test/e2e/internal/helmrelease.go
test/e2e/internal/helmrelease_test.go
test/e2e/internal/postgres.go
test/e2e/internal/postgres_test.go
test/e2e/internal/dex.go
test/e2e/internal/dex_test.go
test/e2e/internal/chart.go
test/e2e/internal/chart_test.go
test/e2e/internal/state.go
test/e2e/internal/environment.go
test/e2e/internal/apiclient.go
test/e2e/e2e_suite_test.go
test/e2e/e2e_test.go
frontend/playwright.config.js
frontend/e2e/login-and-notes.spec.js
.github/workflows/ci.yml
```

---

### Task 1: devcluster CLI skeleton and prerequisite check

**Files:**
- Create: `test/e2e/cmd/devcluster/main.go`
- Create: `test/e2e/internal/prereqs.go`
- Test: `test/e2e/internal/prereqs_test.go`

**Interfaces:**
- Produces: `internal.CheckPrerequisites(lookPath func(string) (string, error)) error`; `internal.RequiredBinaries []string`; a `devcluster` binary with `up`/`down`/`baseurl` subcommands (the latter two filled in by Tasks 7 and 8).

- [ ] **Step 1: Write the failing test**

```go
// test/e2e/internal/prereqs_test.go
package internal_test

import (
	"errors"
	"testing"

	"notebook/test/e2e/internal"
)

func TestCheckPrerequisites_AllPresent_Succeeds(t *testing.T) {
	lookPath := func(bin string) (string, error) { return "/usr/bin/" + bin, nil }
	if err := internal.CheckPrerequisites(lookPath); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckPrerequisites_MissingBinary_ListsIt(t *testing.T) {
	lookPath := func(bin string) (string, error) {
		if bin == "kind" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + bin, nil
	}
	err := internal.CheckPrerequisites(lookPath)
	if err == nil {
		t.Fatalf("expected an error listing the missing binary")
	}
	if !contains(err.Error(), "kind") {
		t.Fatalf("expected error to mention 'kind', got: %v", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/e2e/internal/... -run TestCheckPrerequisites -v`
Expected: FAIL — package `internal` undefined.

- [ ] **Step 3: Implement the prerequisite check**

```go
// test/e2e/internal/prereqs.go
package internal

import "fmt"

var RequiredBinaries = []string{"kubectl", "helm", "kind", "docker"}

func CheckPrerequisites(lookPath func(string) (string, error)) error {
	var missing []string
	for _, bin := range RequiredBinaries {
		if _, err := lookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required binaries: %v", missing)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./test/e2e/internal/... -run TestCheckPrerequisites -v`
Expected: PASS

- [ ] **Step 5: Write the CLI entry point**

```go
// test/e2e/cmd/devcluster/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"notebook/test/e2e/internal"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	if err := internal.CheckPrerequisites(exec.LookPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	switch os.Args[1] {
	case "up":
		if err := internal.Up(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "up failed:", err)
			os.Exit(1)
		}
	case "down":
		deleteCluster := flag.NewFlagSet("down", flag.ExitOnError)
		deleteClusterFlag := deleteCluster.Bool("delete-cluster", false, "also delete the kind cluster")
		_ = deleteCluster.Parse(os.Args[2:])
		if err := internal.Down(ctx, *deleteClusterFlag); err != nil {
			fmt.Fprintln(os.Stderr, "down failed:", err)
			os.Exit(1)
		}
	case "baseurl":
		state, err := internal.LoadState()
		if err != nil {
			fmt.Fprintln(os.Stderr, "no environment is up:", err)
			os.Exit(1)
		}
		fmt.Println(state.BaseURL)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: devcluster <up|down [--delete-cluster]|baseurl>")
}
```

`internal.Up`/`internal.Down` are implemented across Tasks 2–7; this
file won't compile until Task 7 lands `environment.go`. That's expected
— the remaining tasks in this plan build toward it.

- [ ] **Step 6: Commit**

```bash
git add test/e2e/cmd/devcluster test/e2e/internal/prereqs.go test/e2e/internal/prereqs_test.go
git commit -m "feat: add devcluster CLI skeleton and prerequisite binary check"
```

---

### Task 2: Kind cluster lifecycle

**Files:**
- Create: `test/e2e/internal/cluster.go`
- Test: `test/e2e/internal/cluster_test.go`

**Interfaces:**
- Produces: `internal.KindClusterName` (const); `internal.ClusterLister` interface; `internal.EnsureCluster(ctx, lister ClusterLister, name string) error`; `internal.DeleteCluster(ctx, name string) error`.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e/internal/cluster_test.go
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
```

This test only exercises the "already exists, skip creation" branch —
the only one reachable without actually invoking `kind create cluster`,
which Step 3's live verification covers instead.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/e2e/internal/... -run TestEnsureCluster -v`
Expected: FAIL — `internal.ClusterLister`, `internal.EnsureCluster` undefined.

- [ ] **Step 3: Implement cluster lifecycle functions**

```go
// test/e2e/internal/cluster.go
package internal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const KindClusterName = "notebook-e2e"

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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./test/e2e/internal/... -run TestEnsureCluster -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add test/e2e/internal/cluster.go test/e2e/internal/cluster_test.go
git commit -m "feat: add kind cluster lifecycle management"
```

---

### Task 3: Idempotent Helm release installer

**Files:**
- Create: `test/e2e/internal/helmrelease.go`
- Test: `test/e2e/internal/helmrelease_test.go`

**Interfaces:**
- Produces: `internal.HelmRelease{Name, Namespace, Chart, Repo, RepoName, Version string; SetValues []string}`; `internal.ReleaseLister` interface; `internal.EnsureHelmRelease(ctx, lister ReleaseLister, rel HelmRelease) error`; `internal.ClusterDependencies() []HelmRelease` (cert-manager, ingress-nginx, kube-prometheus-stack, CloudNativePG operator); `internal.InstallClusterDependencies(ctx context.Context) error`.

- [ ] **Step 1: Write the failing test**

```go
// test/e2e/internal/helmrelease_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./test/e2e/internal/... -run "TestEnsureHelmRelease|TestClusterDependencies" -v`
Expected: FAIL — types/functions undefined.

- [ ] **Step 3: Implement the installer**

```go
// test/e2e/internal/helmrelease.go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./test/e2e/internal/... -run "TestEnsureHelmRelease|TestClusterDependencies" -v`
Expected: PASS

- [ ] **Step 5: Live verification**

Run: `go run ./test/e2e/cmd/devcluster up` is not yet wired end-to-end
(that's Task 7) — defer full live verification to Task 7's Step. For
now, confirm the package compiles: `go build ./test/e2e/...`.

- [ ] **Step 6: Commit**

```bash
git add test/e2e/internal/helmrelease.go test/e2e/internal/helmrelease_test.go
git commit -m "feat: add idempotent Helm release installer for cluster dependencies"
```

---

### Task 4: Test namespace and CloudNativePG test Postgres cluster

**Files:**
- Create: `test/e2e/internal/postgres.go`
- Test: `test/e2e/internal/postgres_test.go`

**Interfaces:**
- Produces: `internal.TestNamespace` (const); `internal.ApplyManifest(ctx, manifest string) error`; `internal.RenderTestPostgresClusterManifest(namespace string) string`; `internal.EnsureTestNamespace(ctx) error`; `internal.EnsureTestPostgresCluster(ctx) error`; `internal.RenderSelfSignedClusterIssuerManifest() string`; `internal.EnsureSelfSignedClusterIssuer(ctx) error`.

- [ ] **Step 1: Add the YAML test dependency**

```bash
go get gopkg.in/yaml.v3@v3.0.1
```

- [ ] **Step 2: Write the failing tests**

```go
// test/e2e/internal/postgres_test.go
package internal_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"notebook/test/e2e/internal"
)

func TestRenderTestPostgresClusterManifest_IsValidYAMLWithNamespace(t *testing.T) {
	manifest := internal.RenderTestPostgresClusterManifest("my-ns")

	var parsed struct {
		Metadata struct {
			Namespace string `yaml:"namespace"`
			Name      string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Instances int `yaml:"instances"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, manifest)
	}
	if parsed.Metadata.Namespace != "my-ns" {
		t.Fatalf("expected namespace my-ns, got %s", parsed.Metadata.Namespace)
	}
	if parsed.Spec.Instances != 1 {
		t.Fatalf("expected 1 instance, got %d", parsed.Spec.Instances)
	}
}

func TestRenderSelfSignedClusterIssuerManifest_IsValidYAML(t *testing.T) {
	manifest := internal.RenderSelfSignedClusterIssuerManifest()

	var parsed struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, manifest)
	}
	if parsed.Kind != "ClusterIssuer" {
		t.Fatalf("expected kind ClusterIssuer, got %s", parsed.Kind)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./test/e2e/internal/... -run "TestRenderTestPostgresClusterManifest|TestRenderSelfSignedClusterIssuerManifest" -v`
Expected: FAIL — functions undefined.

- [ ] **Step 4: Implement the manifests and apply helpers**

```go
// test/e2e/internal/postgres.go
package internal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const TestNamespace = "notebook-e2e"

func ApplyManifest(ctx context.Context, manifest string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
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

const testPostgresClusterManifestTemplate = `
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: notebook-e2e-postgres
  namespace: %s
spec:
  instances: 1
  storage:
    size: 1Gi
  bootstrap:
    initdb:
      database: notebook
      owner: notebook
`

func RenderTestPostgresClusterManifest(namespace string) string {
	return fmt.Sprintf(testPostgresClusterManifestTemplate, namespace)
}

func EnsureTestPostgresCluster(ctx context.Context) error {
	if err := ApplyManifest(ctx, RenderTestPostgresClusterManifest(TestNamespace)); err != nil {
		return fmt.Errorf("applying test Postgres Cluster: %w", err)
	}
	wait := exec.CommandContext(ctx, "kubectl", "wait", "--for=condition=Ready",
		"cluster/notebook-e2e-postgres", "-n", TestNamespace, "--timeout=5m")
	if err := wait.Run(); err != nil {
		return fmt.Errorf("waiting for test Postgres Cluster to become ready: %w", err)
	}
	return nil
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./test/e2e/internal/... -run "TestRenderTestPostgresClusterManifest|TestRenderSelfSignedClusterIssuerManifest" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add test/e2e/internal/postgres.go test/e2e/internal/postgres_test.go go.mod go.sum
git commit -m "feat: add test namespace, CNPG test Cluster, and self-signed ClusterIssuer manifests"
```

---

### Task 5: Dex test OIDC provider

**Files:**
- Create: `test/e2e/internal/dex.go`
- Test: `test/e2e/internal/dex_test.go`

**Interfaces:**
- Produces: `internal.TestAdminEmail`, `internal.TestUserEmail`, `internal.TestPassword` (consts); `internal.DexConfig{Issuer, ClientID, ClientSecret, RedirectURL string}`; `internal.RenderDexConfig(cfg DexConfig) (string, error)`; `internal.RenderDexManifests(namespace string, configYAML string) string`; `internal.EnsureDex(ctx context.Context, cfg DexConfig) error`.

- [ ] **Step 1: Add the bcrypt dependency**

```bash
go get golang.org/x/crypto@v0.28.0
```

- [ ] **Step 2: Write the failing test**

```go
// test/e2e/internal/dex_test.go
package internal_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"notebook/test/e2e/internal"
)

func TestRenderDexConfig_ProducesValidYAMLWithWorkingPasswordHash(t *testing.T) {
	cfg := internal.DexConfig{
		Issuer:       "http://dex.notebook-e2e.svc.cluster.local:5556",
		ClientID:     "notebook",
		ClientSecret: "notebook-secret",
		RedirectURL:  "http://notebook.notebook-e2e.svc.cluster.local/auth/callback",
	}

	rendered, err := internal.RenderDexConfig(cfg)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	var parsed struct {
		Issuer        string `yaml:"issuer"`
		StaticClients []struct {
			ID           string   `yaml:"id"`
			RedirectURIs []string `yaml:"redirectURIs"`
		} `yaml:"staticClients"`
		StaticPasswords []struct {
			Email string `yaml:"email"`
			Hash  string `yaml:"hash"`
		} `yaml:"staticPasswords"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, rendered)
	}

	if parsed.Issuer != cfg.Issuer {
		t.Fatalf("expected issuer %s, got %s", cfg.Issuer, parsed.Issuer)
	}
	if len(parsed.StaticClients) != 1 || parsed.StaticClients[0].ID != cfg.ClientID {
		t.Fatalf("expected one static client with id %s, got %+v", cfg.ClientID, parsed.StaticClients)
	}
	if len(parsed.StaticPasswords) != 2 {
		t.Fatalf("expected 2 static passwords, got %d", len(parsed.StaticPasswords))
	}
	if err := bcrypt.CompareHashAndPassword([]byte(parsed.StaticPasswords[0].Hash), []byte(internal.TestPassword)); err != nil {
		t.Fatalf("expected rendered hash to verify against TestPassword: %v", err)
	}
}

func TestRenderDexManifests_ProducesValidMultiDocYAML(t *testing.T) {
	manifests := internal.RenderDexManifests("my-ns", "issuer: http://x\n")
	docs := splitYAMLDocs(manifests)
	if len(docs) < 3 {
		t.Fatalf("expected at least 3 documents (ConfigMap, Deployment, Service), got %d", len(docs))
	}
	for _, doc := range docs {
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Fatalf("invalid yaml document: %v\n%s", err, doc)
		}
	}
}

func splitYAMLDocs(s string) []string {
	var docs []string
	for _, part := range splitOn(s, "\n---\n") {
		trimmed := trimSpace(part)
		if trimmed != "" {
			docs = append(docs, trimmed)
		}
	}
	return docs
}

func splitOn(s, sep string) []string {
	var parts []string
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			parts = append(parts, s)
			return parts
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
	}
}

func indexOf(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./test/e2e/internal/... -run "TestRenderDexConfig|TestRenderDexManifests" -v`
Expected: FAIL — `internal.DexConfig`, `internal.RenderDexConfig`,
`internal.RenderDexManifests` undefined.

- [ ] **Step 4: Implement Dex config/manifest rendering and the ensure function**

Note on `TestAdminEmail`/`OIDC_ADMIN_CLAIM`: Dex's static-password
connector (`enablePasswordDB`) does not emit a `groups` claim for local
users without wiring up a separate connector that supports groups,
which would add real complexity to a throwaway test IdP. Since Plan 2's
`ExtractRole` accepts *any* claim name/value pair, this e2e environment
instead maps admin-ness to the `email` claim
(`OIDC_ADMIN_CLAIM=email`, `OIDC_ADMIN_CLAIM_VALUE=admin@notebook.test`)
— a deliberate simplification of the test harness, not a statement
about how a real production IdP should be configured (a real IdP would
use `groups`, as Plan 2's unit tests already exercise with a fake IdP
that does emit array-valued claims).

```go
// test/e2e/internal/dex.go
package internal

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	TestAdminEmail = "admin@notebook.test"
	TestUserEmail  = "user@notebook.test"
	TestPassword   = "notebook-e2e-password"
)

type DexConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

const dexConfigTemplate = `
issuer: %s
storage:
  type: memory
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
staticClients:
  - id: %s
    secret: %s
    redirectURIs:
      - %s
enablePasswordDB: true
staticPasswords:
  - email: %q
    hash: %q
    username: "admin"
    userID: "admin-test-user"
  - email: %q
    hash: %q
    username: "user"
    userID: "regular-test-user"
`

func RenderDexConfig(cfg DexConfig) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(TestPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing test password: %w", err)
	}
	return fmt.Sprintf(dexConfigTemplate,
		cfg.Issuer, cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL,
		TestAdminEmail, string(hash),
		TestUserEmail, string(hash),
	), nil
}

const dexManifestsTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: dex-config
  namespace: %[1]s
data:
  config.yaml: |
%[2]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dex
  namespace: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dex
  template:
    metadata:
      labels:
        app: dex
    spec:
      containers:
        - name: dex
          image: ghcr.io/dexidp/dex:v2.41.1
          args: ["dex", "serve", "/etc/dex/config.yaml"]
          ports:
            - containerPort: 5556
          volumeMounts:
            - name: config
              mountPath: /etc/dex
      volumes:
        - name: config
          configMap:
            name: dex-config
---
apiVersion: v1
kind: Service
metadata:
  name: dex
  namespace: %[1]s
spec:
  selector:
    app: dex
  ports:
    - port: 5556
      targetPort: 5556
`

func indentYAML(s, prefix string) string {
	out := prefix
	for _, c := range s {
		out += string(c)
		if c == '\n' {
			out += prefix
		}
	}
	return out
}

func RenderDexManifests(namespace, configYAML string) string {
	return fmt.Sprintf(dexManifestsTemplate, namespace, indentYAML(configYAML, "    "))
}

func EnsureDex(ctx context.Context, cfg DexConfig) error {
	configYAML, err := RenderDexConfig(cfg)
	if err != nil {
		return fmt.Errorf("rendering Dex config: %w", err)
	}
	manifests := RenderDexManifests(TestNamespace, configYAML)
	if err := ApplyManifest(ctx, manifests); err != nil {
		return fmt.Errorf("applying Dex manifests: %w", err)
	}
	return RolloutWait(ctx, TestNamespace, "deployment/dex")
}
```

Add `RolloutWait` as a small shared helper (used again by Task 6):

```go
// append to test/e2e/internal/postgres.go, or a new tiny file
// test/e2e/internal/kubectl.go — placed there since it's a
// kubectl-wait wrapper, not Postgres-specific.
```

```go
// test/e2e/internal/kubectl.go
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./test/e2e/internal/... -run "TestRenderDexConfig|TestRenderDexManifests" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add test/e2e/internal/dex.go test/e2e/internal/dex_test.go test/e2e/internal/kubectl.go go.mod go.sum
git commit -m "feat: add Dex test OIDC provider config/manifest rendering"
```

---

### Task 6: Image build/load, app secrets, chart install, port-forward

**Files:**
- Create: `test/e2e/internal/chart.go`
- Test: `test/e2e/internal/chart_test.go`

**Interfaces:**
- Produces: `internal.BuildAndLoadImage(ctx, tag string) error`; `internal.EnsureAppSecrets(ctx, dex DexConfig, sessionHashKey, sessionBlockKey []byte) error`; `internal.InstallChart(ctx, imageTag string, dex DexConfig) error`; `internal.StartPortForward(ctx) (pid int, err error)`; `internal.StopPortForward(pid int) error`.

- [ ] **Step 1: Write the failing test for the pure image-tag-splitting logic**

```go
// test/e2e/internal/chart_test.go
package internal_test

import (
	"testing"

	"notebook/test/e2e/internal"
)

func TestSplitImageTag(t *testing.T) {
	cases := []struct {
		in, repo, tag string
	}{
		{"ghcr.io/example/notebook:abc123", "ghcr.io/example/notebook", "abc123"},
		{"notebook:dev", "notebook", "dev"},
		{"notebook", "notebook", "latest"},
	}
	for _, c := range cases {
		repo, tag := internal.SplitImageTag(c.in)
		if repo != c.repo || tag != c.tag {
			t.Fatalf("SplitImageTag(%q) = (%q, %q), want (%q, %q)", c.in, repo, tag, c.repo, c.tag)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/e2e/internal/... -run TestSplitImageTag -v`
Expected: FAIL — `internal.SplitImageTag` undefined.

- [ ] **Step 3: Implement the chart/image/secrets/port-forward helpers**

```go
// test/e2e/internal/chart.go
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
		"upgrade", "--install", "notebook-e2e", "deploy/helm/notebook",
		"-n", TestNamespace, "--create-namespace", "--wait",
		"--set", "image.repository=" + repo,
		"--set", "image.tag=" + tag,
		"--set", "ingress.className=nginx",
		"--set", "ingress.host=notebook.notebook-e2e.svc.cluster.local",
		"--set", "ingress.tls.clusterIssuer=selfsigned-e2e",
		"--set", "postgresql.credentialsSecretRef.name=notebook-e2e-postgres-app",
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./test/e2e/internal/... -run TestSplitImageTag -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add test/e2e/internal/chart.go test/e2e/internal/chart_test.go
git commit -m "feat: add image build/load, app secrets, chart install, and port-forward helpers"
```

---

### Task 7: Environment orchestration (Up/Down) and state persistence

**Files:**
- Create: `test/e2e/internal/state.go`
- Create: `test/e2e/internal/environment.go`

**Interfaces:**
- Consumes: everything from Tasks 2–6.
- Produces: `internal.State{BaseURL, DexIssuer string; PortForwardPID int}`; `internal.SaveState(s State) error`; `internal.LoadState() (State, error)`; `internal.Up(ctx context.Context) error`; `internal.Down(ctx context.Context, deleteCluster bool) error`.

- [ ] **Step 1: Implement state persistence**

```go
// test/e2e/internal/state.go
package internal

import (
	"encoding/json"
	"os"
)

const stateFilePath = ".devcluster-state.json"

type State struct {
	BaseURL        string `json:"base_url"`
	DexIssuer      string `json:"dex_issuer"`
	PortForwardPID int    `json:"port_forward_pid"`
}

func SaveState(s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath, data, 0o600)
}

func LoadState() (State, error) {
	var s State
	data, err := os.ReadFile(stateFilePath)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func ClearState() error {
	err := os.Remove(stateFilePath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
```

- [ ] **Step 2: Implement `Up` and `Down`**

```go
// test/e2e/internal/environment.go
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
// cluster dependencies, the test namespace, CNPG test Postgres, Dex,
// the app's secrets, the built image, the real Helm chart, and a
// port-forward to reach it. Steps run in strict order; each is
// individually idempotent (see Tasks 2-6), so re-running Up after a
// partial failure is safe.
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
	if err := EnsureTestPostgresCluster(ctx); err != nil {
		return fmt.Errorf("ensuring test Postgres cluster: %w", err)
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

	pid, err := StartPortForward(ctx)
	if err != nil {
		return fmt.Errorf("starting port-forward: %w", err)
	}

	return SaveState(State{BaseURL: appBaseURL, DexIssuer: dexIssuer, PortForwardPID: pid})
}

// Down tears down the test namespace (Dex, the chart release, the
// test Postgres Cluster) in reverse order, stopping the port-forward
// first. It leaves cert-manager, the ingress controller, the
// Prometheus Operator, the CNPG operator, and the kind cluster itself
// running, unless deleteCluster is true.
func Down(ctx context.Context, deleteCluster bool) error {
	state, err := LoadState()
	if err == nil && state.PortForwardPID != 0 {
		_ = StopPortForward(state.PortForwardPID)
	}

	if err := runHelmUninstall(ctx, "notebook-e2e", TestNamespace); err != nil {
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
```

```go
// append to test/e2e/internal/kubectl.go
func runHelmUninstall(ctx context.Context, release, namespace string) error {
	cmd := exec.CommandContext(ctx, "helm", "uninstall", release, "-n", namespace, "--ignore-not-found")
	return cmd.Run()
}

func runKubectlDelete(ctx context.Context, kind, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", kind, name, "--ignore-not-found", "--wait")
	return cmd.Run()
}
```

Add `"context"` and `"os/exec"` to `kubectl.go`'s imports if not
already present (they are, from `RolloutWait`).

- [ ] **Step 3: Confirm the whole `test/e2e` tree builds**

Run: `go build ./test/e2e/...`
Expected: succeeds (this is the first point `main.go`'s calls to
`internal.Up`/`internal.Down` resolve).

- [ ] **Step 4: Live verification against a real kind cluster**

Run: `go run ./test/e2e/cmd/devcluster up`
Expected: completes without error, standing up the full environment
described above (this takes several minutes on first run).

Run: `kubectl get pods -n notebook-e2e` and confirm the Dex, Postgres,
and notebook app pods are all `Running`/`Ready`.

Run: `curl -s http://127.0.0.1:18080/healthz`
Expected: `{"status":"ok"}`

Run: `go run ./test/e2e/cmd/devcluster down`
Expected: completes without error; `kubectl get ns notebook-e2e`
afterward reports the namespace is gone or terminating.

- [ ] **Step 5: Commit**

```bash
git add test/e2e/internal/state.go test/e2e/internal/environment.go test/e2e/internal/kubectl.go
git commit -m "feat: add Up/Down environment orchestration and state persistence"
```

---

### Task 8: Ginkgo e2e suite

**Files:**
- Create: `test/e2e/internal/apiclient.go`
- Create: `test/e2e/e2e_suite_test.go`
- Create: `test/e2e/e2e_test.go`

**Interfaces:**
- Consumes: `internal.Up`/`Down`/`LoadState` (Task 7).
- Produces: a `go test -tags e2e ./test/e2e/...` suite.

- [ ] **Step 1: Add Ginkgo/Gomega dependencies**

```bash
go get github.com/onsi/ginkgo/v2@v2.20.2
go get github.com/onsi/gomega@v1.34.2
```

- [ ] **Step 2: Implement the scripted Dex login and API helper client**

```go
// test/e2e/internal/apiclient.go
package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
)

func NewCookieClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

var formActionPattern = regexp.MustCompile(`<form[^>]+action="([^"]+)"`)

// LoginViaDex drives the full browserless login round trip: GET
// /auth/login (which redirects through our backend's OIDC flow to
// Dex's static login form), scrapes that form's action URL, submits
// the given credentials, and follows Dex's redirect back through
// /auth/callback — leaving an authenticated session cookie in
// client's cookie jar.
func LoginViaDex(client *http.Client, baseURL, email, password string) error {
	resp, err := client.Get(baseURL + "/auth/login")
	if err != nil {
		return fmt.Errorf("fetching login page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading login page: %w", err)
	}

	matches := formActionPattern.FindSubmatch(body)
	if matches == nil {
		return fmt.Errorf("could not find login form action in Dex's login page")
	}
	actionURL, err := resp.Request.URL.Parse(string(matches[1]))
	if err != nil {
		return fmt.Errorf("resolving form action URL: %w", err)
	}

	form := url.Values{"login": {email}, "password": {password}}
	submitResp, err := client.PostForm(actionURL.String(), form)
	if err != nil {
		return fmt.Errorf("submitting Dex login form: %w", err)
	}
	defer submitResp.Body.Close()

	if submitResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(submitResp.Body)
		return fmt.Errorf("unexpected status %d after Dex login submission: %s", submitResp.StatusCode, b)
	}
	return nil
}

type NoteSummary struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

func CreateNote(client *http.Client, baseURL, title, content string) (*NoteSummary, error) {
	body, _ := json.Marshal(map[string]string{"title": title, "content": content})
	resp, err := client.Post(baseURL+"/api/notes", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d creating note: %s", resp.StatusCode, b)
	}
	var note NoteSummary
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
		return nil, err
	}
	return &note, nil
}

func ListNotes(client *http.Client, baseURL string) ([]NoteSummary, error) {
	resp, err := client.Get(baseURL + "/api/notes")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Items []NoteSummary `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Items, nil
}
```

- [ ] **Step 3: Write the Ginkgo suite entry point**

```go
// test/e2e/e2e_suite_test.go
//go:build e2e

package e2e_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"notebook/test/e2e/internal"
)

var state internal.State

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notebook E2E Suite")
}

var _ = BeforeSuite(func() {
	Expect(internal.Up(context.Background())).To(Succeed())
	var err error
	state, err = internal.LoadState()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	Expect(internal.Down(context.Background(), false)).To(Succeed())
})
```

- [ ] **Step 4: Write the assertions**

```go
// test/e2e/e2e_test.go
//go:build e2e

package e2e_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"notebook/test/e2e/internal"
)

var _ = Describe("Notebook app", func() {
	It("serves /healthz", func() {
		resp, err := http.Get(state.BaseURL + "/healthz")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("redirects /auth/login to the Dex authorization endpoint", func() {
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		resp, err := client.Get(state.BaseURL + "/auth/login")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		Expect(resp.Header.Get("Location")).To(ContainSubstring(state.DexIssuer))
	})

	It("lets an admin log in via Dex and create a note that appears in the list", func() {
		client := internal.NewCookieClient()
		Expect(internal.LoginViaDex(client, state.BaseURL, internal.TestAdminEmail, internal.TestPassword)).To(Succeed())

		created, err := internal.CreateNote(client, state.BaseURL, "E2E Note", "Hello from e2e")
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Title).To(Equal("E2E Note"))

		notes, err := internal.ListNotes(client, state.BaseURL)
		Expect(err).NotTo(HaveOccurred())
		var titles []string
		for _, n := range notes {
			titles = append(titles, n.Title)
		}
		Expect(titles).To(ContainElement("E2E Note"))
	})
})
```

- [ ] **Step 5: Run the suite**

Run: `go test -tags e2e ./test/e2e/... -v -timeout 30m`
Expected: PASS (this both builds and tears down a full kind
environment as part of the run — expect several minutes).

- [ ] **Step 6: Commit**

```bash
git add test/e2e/internal/apiclient.go test/e2e/e2e_suite_test.go test/e2e/e2e_test.go go.mod go.sum
git commit -m "feat: add Ginkgo e2e suite covering health, OIDC redirect, and login-to-note-creation"
```

---

### Task 9: Playwright frontend e2e suite

**Files:**
- Create: `frontend/playwright.config.js`
- Create: `frontend/e2e/login-and-notes.spec.js`
- Modify: `frontend/package.json`

**Interfaces:** drives the real app through a real Chromium browser
against `E2E_BASE_URL`.

- [ ] **Step 1: Add the Playwright dev dependency and script**

```bash
cd frontend && npm install -D @playwright/test@^1.48.0
```

Add to `frontend/package.json`'s `scripts`:

```json
"test:e2e": "playwright test"
```

- [ ] **Step 2: Write the Playwright config**

```js
// frontend/playwright.config.js
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: process.env.E2E_BASE_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
})
```

- [ ] **Step 3: Write the e2e spec**

```js
// frontend/e2e/login-and-notes.spec.js
import { test, expect } from '@playwright/test'

const adminEmail = process.env.E2E_ADMIN_EMAIL || 'admin@notebook.test'
const password = process.env.E2E_PASSWORD || 'notebook-e2e-password'

test.beforeEach(async ({ page, baseURL }) => {
  if (!baseURL) {
    throw new Error('E2E_BASE_URL must be set to run this suite')
  }
  await page.goto('/notes')
  await page.waitForSelector('input[name="login"]', { timeout: 15000 })
  await page.fill('input[name="login"]', adminEmail)
  await page.fill('input[name="password"]', password)
  await page.click('button[type="submit"]')
  await page.waitForURL('**/notes**')
})

test('creates a note and sees it rendered', async ({ page }) => {
  await page.click('text=New note')
  await page.fill('input[placeholder="Title"]', 'Playwright Note')
  await page.fill('textarea', '# Hello from Playwright')
  await page.click('text=Create')

  await expect(page.locator('h2')).toHaveText('Playwright Note')
})

test('edits and deletes a note', async ({ page }) => {
  await page.click('text=New note')
  await page.fill('input[placeholder="Title"]', 'To Edit')
  await page.fill('textarea', 'original content')
  await page.click('text=Create')

  await page.click('text=Edit')
  await page.fill('input', 'Edited Title')
  await page.click('text=Save')
  await expect(page.locator('h2')).toHaveText('Edited Title')

  await page.click('text=Delete')
  await expect(page.locator('h2')).not.toContainText('Edited Title')
})

test('adds and deletes a comment', async ({ page }) => {
  await page.click('text=New note')
  await page.fill('input[placeholder="Title"]', 'Commented Note')
  await page.fill('textarea', 'content')
  await page.click('text=Create')

  await page.fill('textarea[placeholder="Add a comment..."]', 'Nice note!')
  await page.click('text=Comment')
  await expect(page.locator('text=Nice note!')).toBeVisible()

  await page.click('text=Delete >> nth=-1')
  await expect(page.locator('text=Nice note!')).not.toBeVisible()
})
```

- [ ] **Step 4: Run the suite against a live devcluster environment**

Run: `go run ./test/e2e/cmd/devcluster up`
Run: `cd frontend && npx playwright install --with-deps chromium`
Run: `E2E_BASE_URL=$(go run ../test/e2e/cmd/devcluster baseurl) npm run test:e2e`
Expected: all three specs pass.
Run: `go run ./test/e2e/cmd/devcluster down`

- [ ] **Step 5: Commit**

```bash
git add frontend/playwright.config.js frontend/e2e frontend/package.json frontend/package-lock.json
git commit -m "feat: add Playwright e2e suite for login and note/comment flows"
```

---

### Task 10: GitHub Actions CI wiring

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:** none — this wires together everything built in Plans
1–6.

- [ ] **Step 1: Write the workflow**

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  backend-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
      - run: go build ./...
      - run: go vet ./...
      - run: go test ./...

  frontend-test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: frontend
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: "22"
          cache: "npm"
          cache-dependency-path: frontend/package-lock.json
      - run: npm ci
      - run: npm run test
      - run: npm run build

  helm-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - name: helm lint (default values)
        run: |
          helm lint deploy/helm/notebook \
            --set ingress.className=nginx --set ingress.host=x --set ingress.tls.clusterIssuer=x \
            --set postgresql.credentialsSecretRef.name=x \
            --set oidc.issuerURL=https://x --set oidc.redirectURL=https://x/auth/callback \
            --set oidc.clientIDSecretRef.name=x --set oidc.clientSecretSecretRef.name=x \
            --set session.hashKeySecretRef.name=x --set session.blockKeySecretRef.name=x
      - name: helm template (default values)
        run: |
          helm template deploy/helm/notebook \
            --set ingress.className=nginx --set ingress.host=x --set ingress.tls.clusterIssuer=x \
            --set postgresql.credentialsSecretRef.name=x \
            --set oidc.issuerURL=https://x --set oidc.redirectURL=https://x/auth/callback \
            --set oidc.clientIDSecretRef.name=x --set oidc.clientSecretSecretRef.name=x \
            --set session.hashKeySecretRef.name=x --set session.blockKeySecretRef.name=x > /dev/null
      - name: helm template (backup enabled)
        run: |
          helm template deploy/helm/notebook \
            --set ingress.className=nginx --set ingress.host=x --set ingress.tls.clusterIssuer=x \
            --set postgresql.credentialsSecretRef.name=x \
            --set oidc.issuerURL=https://x --set oidc.redirectURL=https://x/auth/callback \
            --set oidc.clientIDSecretRef.name=x --set oidc.clientSecretSecretRef.name=x \
            --set session.hashKeySecretRef.name=x --set session.blockKeySecretRef.name=x \
            --set postgresql.backup.enabled=true \
            --set postgresql.backup.destinationPath=s3://bucket/path \
            --set postgresql.backup.endpointURL=https://s3.example.com \
            --set postgresql.backup.s3CredentialsSecretRef.name=x \
            --set postgresql.backup.s3CredentialsSecretRef.accessKeyIdKey=access-key \
            --set postgresql.backup.s3CredentialsSecretRef.secretAccessKeyKey=secret-key > /dev/null

  e2e-cluster:
    runs-on: ubuntu-latest
    needs: [backend-test, frontend-test, helm-lint]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
      - uses: actions/setup-node@v4
        with:
          node-version: "22"
      - uses: helm/kind-action@v1
        with:
          install_only: true
      - uses: azure/setup-helm@v4
      - name: Bring up the devcluster environment
        run: go run ./test/e2e/cmd/devcluster up
      - name: Run Ginkgo cluster e2e suite
        run: go test -tags e2e ./test/e2e/... -v -timeout 30m
      - name: Run Playwright frontend e2e suite against the live environment
        working-directory: frontend
        run: |
          npm ci
          npx playwright install --with-deps chromium
          E2E_BASE_URL=$(go run ../test/e2e/cmd/devcluster baseurl) npm run test:e2e
      - name: Upload Playwright report on failure
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: playwright-report
          path: frontend/playwright-report/
      - name: Tear down the devcluster environment
        if: always()
        run: go run ./test/e2e/cmd/devcluster down --delete-cluster
```

- [ ] **Step 2: Push the branch and confirm the workflow runs end-to-end on GitHub Actions**

This step can't be verified locally — it requires pushing to a branch
with a PR open so GitHub Actions picks up the workflow. Confirm all
four jobs (`backend-test`, `frontend-test`, `helm-lint`, `e2e-cluster`)
appear and pass in the Actions tab for that PR.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: wire backend/frontend tests, helm lint, and e2e suites into GitHub Actions"
```

---

## Out of scope for this plan

Nothing further is deferred — this is the last of the six plans. Any
gap discovered during this plan's implementation (e.g. a Dex version
bump changing login form markup, a CNPG secret-naming change) should be
fixed directly in this plan's files and, if it reveals something wrong
in an earlier plan's assumptions, backported as a fix to that plan's
document for posterity — there's no "Plan 7" to defer it to.
