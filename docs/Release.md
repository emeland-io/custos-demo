# Release process

Cutting a release is a single step: push a version tag.

```sh
git tag v1.4.0
git push origin v1.4.0
```

That tag push is the *only* trigger. There is no separate "create a GitHub
Release" step — the [release workflow](../.github/workflows/release.yml)
creates the Release for you as part of the run.

## Tag format

Tags must match `v<MAJOR>.<MINOR>.<PATCH>` exactly (e.g. `v1.4.0`). Anything
else — pre-release suffixes like `v1.4.0-rc1`, missing the `v`, extra
segments — is not matched by the workflow's trigger and will not publish
anything.

## What the workflow does

On a matching tag push, `.github/workflows/release.yml`:

1. Re-runs the same backend/frontend tests and Helm lint checks as regular CI
   (via the shared [test.yml](../.github/workflows/test.yml) workflow), and
   stops here if anything fails. This matters because a tag can point at any
   commit, not necessarily one that already passed CI on `main`.
2. Builds the container image for `linux/amd64` and `linux/arm64` and pushes
   it to GHCR as `ghcr.io/emeland-io/custos-demo`, tagged with both the exact
   version (e.g. `1.4.0`) and `latest`.
3. Packages the Helm chart from [deploy/helm/notebook](../deploy/helm/notebook)
   and pushes it as an OCI artifact to
   `oci://ghcr.io/emeland-io/custos-demo/charts/notebook`, tagged with the
   exact version only (no `latest` — chart installs are expected to pin a
   version explicitly).
4. Creates the GitHub Release itself, with auto-generated release notes, and
   attaches the packaged chart `.tgz` as a release asset.

## Versioning model

The tag is the single source of truth for the version. `vMAJOR.MINOR.PATCH`
becomes:

- the container image tag (`MAJOR.MINOR.PATCH`, plus `latest`)
- the Helm chart's `version` and `appVersion`

**The checked-in [Chart.yaml](../deploy/helm/notebook/Chart.yaml) is not
updated by releases and is not the real version.** It stays at its
placeholder value (currently `0.1.0`) as a development default. The real
version is injected at publish time via `helm package --version
--app-version`, not committed back to the repo. There is deliberately no
release-driven commit back to `main` — it would otherwise have to race
against whatever else is being pushed at release time.

## Consuming a release

Pull the image directly:

```sh
docker pull ghcr.io/emeland-io/custos-demo:1.4.0
```

Install the chart straight from the OCI registry:

```sh
helm install notebook oci://ghcr.io/emeland-io/custos-demo/charts/notebook --version 1.4.0
```

...or from the `.tgz` attached to the GitHub Release, without touching GHCR
at all:

```sh
helm install notebook https://github.com/emeland-io/custos-demo/releases/download/v1.4.0/notebook-1.4.0.tgz
```

## One-time manual setup: package visibility

GHCR packages default to **private** the first time they're pushed, even
though this repository is public. Until this is fixed, `docker pull` /
`helm pull` without authentication will fail for anyone outside the
`emeland-io` org — the push itself will still succeed.

**This should already have been done** after the first release, but if
image or chart pulls start failing with an auth error, check it:

1. Go to the package's page — from the org's package list at
   `https://github.com/orgs/emeland-io/packages`, or via the "Packages" link
   in this repo's right-hand sidebar on GitHub.
2. Open `custos-demo` (the image) and separately `custos-demo/charts/notebook`
   (the chart) — these are two independent packages and each needs this done.
3. On the package page, click **Package settings** (bottom of the right
   sidebar).
4. Under **Danger Zone**, use **Change visibility** and set it to **Public**.
5. While there, under **Manage Actions access** / **Connect repository**,
   confirm it's linked to `emeland-io/custos-demo` so permissions stay in
   sync with the source repo.

This is a one-time step per package, not a per-release step — once public,
it stays public across future version pushes.

## Scope notes

Multi-arch builds, test gating, and auto-generated release notes are in
scope today. Image signing / SBOM attestation (e.g. `cosign` + GitHub OIDC
keyless signing) is not — it's a reasonable follow-up hardening step, but a
separate one from standing up the basic publish pipeline.
