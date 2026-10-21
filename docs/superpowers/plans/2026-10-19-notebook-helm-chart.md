# Notebook App — Helm Chart & Kubernetes Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Helm chart that deploys the notebook app: a
Deployment/Service/Ingress for the app itself, a CloudNativePG
`Cluster` custom resource for Postgres, a pre-upgrade migration hook
Job, and values files for default/dev/prod environments — all assuming
cert-manager, an ingress controller, the CloudNativePG operator, and
the Prometheus Operator are already installed on the cluster (this
chart documents that requirement; it does not install any of them).

**Architecture:** Standard Helm chart layout under
`deploy/helm/notebook/`. `values.yaml` is the single source of
configurable knobs (image, replica count, ingress class/issuer,
Postgres instance count/storage, secret references, optional backup
config). Templates render conditionally off those values rather than
hardcoding environment-specific choices, so `values-dev.yaml` and
`values-prod.yaml` only need to override the handful of values that
actually differ.

**Tech Stack:** Helm 3 chart templates (YAML + Go templating),
CloudNativePG CRDs (`postgresql.cnpg.io/v1`), a `PodMonitor` or
annotation-based scrape config for the Prometheus Operator, standard
`networking.k8s.io/v1` Ingress.

**Spec:** `docs/superpowers/specs/2026-10-19-notebook-app.md`

## Handoff Note (read before starting)

This plan assumes Plan 4
(`docs/superpowers/plans/2026-10-19-notebook-container-build.md`) was
implemented exactly as written: the image exposes port `8080`, serves
`/healthz` and `/metrics`, and is configured entirely via the
environment variables in `internal/config/config.go`
(`DATABASE_URL`, `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`,
`OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL`, `OIDC_ADMIN_CLAIM`,
`OIDC_ADMIN_CLAIM_VALUE`, `SESSION_HASH_KEY`, `SESSION_BLOCK_KEY`).
**Before this plan's first task starts, the reviewer who signed off on
Plan 4's final whole-branch review must re-read this plan against the
actual Plan 1–4 codebase** (env var names, port, probe paths, the
goose migration invocation Plan 1 wired up) and correct any drift
before proceeding.

## Global Constraints

- The chart never installs cert-manager, an ingress controller, the
  CloudNativePG operator, or the Prometheus Operator — it only assumes
  they exist and documents that assumption in the chart's `README.md`.
- The chart never creates Kubernetes `Secret` objects; every secret
  value is referenced by `{name, key}` via values, so the deploying
  org can manage secrets however they already do (manual `kubectl
  create secret`, sealed-secrets, External Secrets Operator, Vault).
- `ingress.className` and `ingress.tls.clusterIssuer` have no default
  value in `values.yaml` — `helm template`/`helm install` must fail
  loudly (via Helm's `required` function) if they're left unset, not
  silently point at a cluster issuer that doesn't exist.
- Default `replicaCount: 1`, no `HorizontalPodAutoscaler` resource is
  templated in this version of the chart.
- Default CNPG `instances: 1`, default storage `5Gi`; HA is reachable
  purely via a values override (`postgresql.instances: 3`), no
  template change required.
- CNPG backup/Barman configuration is off by default and only rendered
  into the `Cluster` spec when `postgresql.backup.enabled` is `true`.
- Every template must render cleanly under `helm lint` and
  `helm template` with each of `values.yaml`, `values-dev.yaml`, and
  `values-prod.yaml` — these are exactly the checks Plan 6's CI wiring
  will run on every PR.

## Review Focus

- `helm template` with `ingress.className` or `ingress.tls.clusterIssuer`
  left unset in the values used — must fail with a clear "values entry
  required" error, not render an Ingress with an empty string that
  Kubernetes would then reject at apply time with a less clear error.
- A values override that sets `postgresql.backup.enabled: true` without
  also supplying the S3 bucket/credentials-secret values — must fail
  chart rendering with a clear message, not render a `Cluster` CR with
  a half-populated backup block that CNPG would reject.
- The migration hook Job — must be annotated so Helm runs it
  pre-upgrade and deletes/replaces it on every release (not left
  dangling as a completed Job that blocks the next `helm upgrade` with
  a name collision).
- `helm uninstall` — must not delete the CNPG `Cluster` CR's underlying
  PVC/data by default (CNPG's own reclaim behavior governs this, but
  the chart must not add `helm.sh/resource-policy: keep` in a way that
  contradicts that, nor remove such protection if CNPG relies on it).
- Setting `replicaCount` to a value greater than 1 — the Deployment
  template must render that many replicas correctly (not hardcode `1`
  anywhere it should read from `.Values.replicaCount`).

---

## File Structure

```
deploy/helm/notebook/Chart.yaml
deploy/helm/notebook/values.yaml
deploy/helm/notebook/values-dev.yaml
deploy/helm/notebook/values-prod.yaml
deploy/helm/notebook/README.md
deploy/helm/notebook/templates/_helpers.tpl
deploy/helm/notebook/templates/deployment.yaml
deploy/helm/notebook/templates/service.yaml
deploy/helm/notebook/templates/ingress.yaml
deploy/helm/notebook/templates/postgres-cluster.yaml
deploy/helm/notebook/templates/migration-job.yaml
deploy/helm/notebook/templates/podmonitor.yaml
deploy/helm/notebook/templates/tests/test-connection.yaml
```

---

### Task 1: Chart skeleton, values schema, and helper templates

**Files:**
- Create: `deploy/helm/notebook/Chart.yaml`
- Create: `deploy/helm/notebook/values.yaml`
- Create: `deploy/helm/notebook/templates/_helpers.tpl`

**Interfaces:**
- Produces: chart name `notebook`; helper templates `notebook.fullname`, `notebook.labels`, `notebook.selectorLabels`, `notebook.secretRefEnv` (renders a single `env:` entry from a `{name, key}` values map).

- [ ] **Step 1: Write `Chart.yaml`**

```yaml
# deploy/helm/notebook/Chart.yaml
apiVersion: v2
name: notebook
description: Internal team notebook app (notes with Markdown and comments)
type: application
version: 0.1.0
appVersion: "0.1.0"
```

- [ ] **Step 2: Write the values schema**

```yaml
# deploy/helm/notebook/values.yaml
replicaCount: 1

image:
  repository: ghcr.io/example/notebook
  tag: ""  # defaults to .Chart.AppVersion when empty
  pullPolicy: IfNotPresent

service:
  port: 8080

ingress:
  className: ""      # required — e.g. "nginx"; no default on purpose
  host: ""            # required — e.g. "notebook.example.com"
  tls:
    clusterIssuer: "" # required — e.g. "letsencrypt-prod"

resources: {}

oidc:
  issuerURL: ""
  redirectURL: ""
  adminClaim: "groups"
  adminClaimValue: "notebook-admin"
  clientIDSecretRef:
    name: ""
    key: "client-id"
  clientSecretSecretRef:
    name: ""
    key: "client-secret"

session:
  hashKeySecretRef:
    name: ""
    key: "hash-key"
  blockKeySecretRef:
    name: ""
    key: "block-key"

postgresql:
  instances: 1
  storageSize: 5Gi
  credentialsSecretRef:
    name: "" # must contain a "uri" key with a full postgres:// DSN
    key: "uri"
  backup:
    enabled: false
    # Required when enabled:
    # destinationPath: "s3://my-bucket/notebook-backups"
    # endpointURL: "https://s3.example.com"
    # s3CredentialsSecretRef: {name: "", accessKeyIdKey: "", secretAccessKeyKey: ""}
```

- [ ] **Step 3: Write the helper templates**

```yaml
{{/* deploy/helm/notebook/templates/_helpers.tpl */}}
{{- define "notebook.fullname" -}}
{{- .Release.Name }}-notebook
{{- end -}}

{{- define "notebook.labels" -}}
app.kubernetes.io/name: notebook
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "notebook.selectorLabels" -}}
app.kubernetes.io/name: notebook
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
```

- [ ] **Step 4: Verify the chart skeleton lints**

Run: `helm lint deploy/helm/notebook --set ingress.className=nginx --set ingress.host=notebook.example.com --set ingress.tls.clusterIssuer=letsencrypt-prod`
Expected: `0 chart(s) linted, 0 chart(s) failed` (no templates reference
the new values yet, so this just validates `Chart.yaml`/`values.yaml`
syntax).

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/notebook/Chart.yaml deploy/helm/notebook/values.yaml deploy/helm/notebook/templates/_helpers.tpl
git commit -m "feat: add Helm chart skeleton with values schema and helper templates"
```

---

### Task 2: Deployment, Service, and Ingress templates

**Files:**
- Create: `deploy/helm/notebook/templates/deployment.yaml`
- Create: `deploy/helm/notebook/templates/service.yaml`
- Create: `deploy/helm/notebook/templates/ingress.yaml`

**Interfaces:**
- Consumes: helper templates (Task 1), `values.yaml` schema (Task 1).

- [ ] **Step 1: Write the Deployment template**

```yaml
# deploy/helm/notebook/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "notebook.fullname" . }}
  labels:
    {{- include "notebook.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "notebook.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "notebook.selectorLabels" . | nindent 8 }}
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "{{ .Values.service.port }}"
        prometheus.io/path: "/metrics"
    spec:
      containers:
        - name: notebook
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: {{ .Values.service.port }}
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: {{ required "postgresql.credentialsSecretRef.name is required" .Values.postgresql.credentialsSecretRef.name }}
                  key: {{ .Values.postgresql.credentialsSecretRef.key }}
            - name: OIDC_ISSUER_URL
              value: {{ required "oidc.issuerURL is required" .Values.oidc.issuerURL | quote }}
            - name: OIDC_REDIRECT_URL
              value: {{ required "oidc.redirectURL is required" .Values.oidc.redirectURL | quote }}
            - name: OIDC_ADMIN_CLAIM
              value: {{ .Values.oidc.adminClaim | quote }}
            - name: OIDC_ADMIN_CLAIM_VALUE
              value: {{ .Values.oidc.adminClaimValue | quote }}
            - name: OIDC_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: {{ required "oidc.clientIDSecretRef.name is required" .Values.oidc.clientIDSecretRef.name }}
                  key: {{ .Values.oidc.clientIDSecretRef.key }}
            - name: OIDC_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: {{ required "oidc.clientSecretSecretRef.name is required" .Values.oidc.clientSecretSecretRef.name }}
                  key: {{ .Values.oidc.clientSecretSecretRef.key }}
            - name: SESSION_HASH_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ required "session.hashKeySecretRef.name is required" .Values.session.hashKeySecretRef.name }}
                  key: {{ .Values.session.hashKeySecretRef.key }}
            - name: SESSION_BLOCK_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ required "session.blockKeySecretRef.name is required" .Values.session.blockKeySecretRef.name }}
                  key: {{ .Values.session.blockKeySecretRef.key }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
```

- [ ] **Step 2: Write the Service template**

```yaml
# deploy/helm/notebook/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "notebook.fullname" . }}
  labels:
    {{- include "notebook.labels" . | nindent 4 }}
spec:
  selector:
    {{- include "notebook.selectorLabels" . | nindent 4 }}
  ports:
    - name: http
      port: {{ .Values.service.port }}
      targetPort: http
```

- [ ] **Step 3: Write the Ingress template**

```yaml
# deploy/helm/notebook/templates/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "notebook.fullname" . }}
  labels:
    {{- include "notebook.labels" . | nindent 4 }}
  annotations:
    cert-manager.io/cluster-issuer: {{ required "ingress.tls.clusterIssuer is required" .Values.ingress.tls.clusterIssuer | quote }}
spec:
  ingressClassName: {{ required "ingress.className is required" .Values.ingress.className | quote }}
  tls:
    - hosts:
        - {{ required "ingress.host is required" .Values.ingress.host | quote }}
      secretName: {{ include "notebook.fullname" . }}-tls
  rules:
    - host: {{ .Values.ingress.host | quote }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ include "notebook.fullname" . }}
                port:
                  name: http
```

- [ ] **Step 4: Verify required-value enforcement and successful rendering**

Run: `helm template deploy/helm/notebook`
Expected: fails with an error naming the first missing `required`
value (e.g. `postgresql.credentialsSecretRef.name is required`).

Run:
```bash
helm template deploy/helm/notebook \
  --set ingress.className=nginx \
  --set ingress.host=notebook.example.com \
  --set ingress.tls.clusterIssuer=letsencrypt-prod \
  --set postgresql.credentialsSecretRef.name=notebook-db \
  --set oidc.issuerURL=https://idp.example.com \
  --set oidc.redirectURL=https://notebook.example.com/auth/callback \
  --set oidc.clientIDSecretRef.name=notebook-oidc \
  --set oidc.clientSecretSecretRef.name=notebook-oidc \
  --set session.hashKeySecretRef.name=notebook-session \
  --set session.blockKeySecretRef.name=notebook-session
```
Expected: renders a complete Deployment/Service/Ingress with no errors.

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/notebook/templates/deployment.yaml deploy/helm/notebook/templates/service.yaml deploy/helm/notebook/templates/ingress.yaml
git commit -m "feat: add Deployment, Service, and Ingress templates"
```

---

### Task 3: CloudNativePG Cluster template

**Files:**
- Create: `deploy/helm/notebook/templates/postgres-cluster.yaml`

**Interfaces:**
- Consumes: `values.yaml`'s `postgresql.*` keys (Task 1).

- [ ] **Step 1: Write the CNPG Cluster template**

```yaml
# deploy/helm/notebook/templates/postgres-cluster.yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: {{ include "notebook.fullname" . }}-postgres
  labels:
    {{- include "notebook.labels" . | nindent 4 }}
spec:
  instances: {{ .Values.postgresql.instances }}
  storage:
    size: {{ .Values.postgresql.storageSize }}
  bootstrap:
    initdb:
      database: notebook
      owner: notebook
  {{- if .Values.postgresql.backup.enabled }}
  backup:
    barmanObjectStore:
      destinationPath: {{ required "postgresql.backup.destinationPath is required when postgresql.backup.enabled is true" .Values.postgresql.backup.destinationPath | quote }}
      endpointURL: {{ required "postgresql.backup.endpointURL is required when postgresql.backup.enabled is true" .Values.postgresql.backup.endpointURL | quote }}
      s3Credentials:
        accessKeyId:
          name: {{ required "postgresql.backup.s3CredentialsSecretRef.name is required when postgresql.backup.enabled is true" .Values.postgresql.backup.s3CredentialsSecretRef.name }}
          key: {{ required "postgresql.backup.s3CredentialsSecretRef.accessKeyIdKey is required when postgresql.backup.enabled is true" .Values.postgresql.backup.s3CredentialsSecretRef.accessKeyIdKey }}
        secretAccessKey:
          name: {{ .Values.postgresql.backup.s3CredentialsSecretRef.name }}
          key: {{ required "postgresql.backup.s3CredentialsSecretRef.secretAccessKeyKey is required when postgresql.backup.enabled is true" .Values.postgresql.backup.s3CredentialsSecretRef.secretAccessKeyKey }}
  {{- end }}
```

- [ ] **Step 2: Verify default rendering (backup disabled) and backup-enabled rendering**

Run (reusing the full `--set` list from Task 2 Step 4):
```bash
helm template deploy/helm/notebook \
  --set ingress.className=nginx --set ingress.host=notebook.example.com --set ingress.tls.clusterIssuer=letsencrypt-prod \
  --set postgresql.credentialsSecretRef.name=notebook-db \
  --set oidc.issuerURL=https://idp.example.com --set oidc.redirectURL=https://notebook.example.com/auth/callback \
  --set oidc.clientIDSecretRef.name=notebook-oidc --set oidc.clientSecretSecretRef.name=notebook-oidc \
  --set session.hashKeySecretRef.name=notebook-session --set session.blockKeySecretRef.name=notebook-session \
  -s templates/postgres-cluster.yaml
```
Expected: a `Cluster` with `instances: 1`, `storage.size: 5Gi`, no
`backup` block.

Run the same command plus
`--set postgresql.backup.enabled=true` (no backup destination/endpoint
set): expected to fail with the `postgresql.backup.destinationPath is
required...` error.

Run it again additionally with
`--set postgresql.backup.destinationPath=s3://bucket/path --set postgresql.backup.endpointURL=https://s3.example.com --set postgresql.backup.s3CredentialsSecretRef.name=notebook-backup --set postgresql.backup.s3CredentialsSecretRef.accessKeyIdKey=access-key --set postgresql.backup.s3CredentialsSecretRef.secretAccessKeyKey=secret-key`:
expected to render a complete `backup.barmanObjectStore` block.

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/notebook/templates/postgres-cluster.yaml
git commit -m "feat: add CloudNativePG Cluster template with optional backup config"
```

---

### Task 4: Migration pre-upgrade hook Job

**Files:**
- Create: `deploy/helm/notebook/templates/migration-job.yaml`

**Interfaces:**
- Consumes: same image/env values as the Deployment template (Task 2); runs the same binary with a `--migrate-only` style invocation — see Step 1 note on why this requires a small, intentional addition to `cmd/server/main.go` beyond what Plan 1/4 built.

- [ ] **Step 1: Note the required `cmd/server/main.go` addition**

Plans 1–4 run `migrations.Up` unconditionally at the top of `main()`
every time the server process starts, which is fine for a single
long-running process but wrong for a dedicated migration Job: the Job
needs to run migrations *and then exit*, not go on to start an HTTP
server that never gets traffic (a Job's pod would then "complete"
successfully by crashing when `ListenAndServe` eventually errors, or
hang forever if it didn't). Add a `--migrate-only` flag to
`cmd/server/main.go`:

```go
// cmd/server/main.go — add near the top of main(), after cfg, err := config.Load()
migrateOnly := flag.Bool("migrate-only", false, "run database migrations then exit")
flag.Parse()
```

and, immediately after the existing `migrations.Up(ctx, pool)` call:

```go
if *migrateOnly {
	logger.Info("migrations applied, exiting due to --migrate-only")
	return
}
```

Add `"flag"` to the import block. Run `go test ./... -v` afterward to
confirm this change doesn't break any existing test (none of them
invoke `main()` directly, so none are affected).

- [ ] **Step 2: Write the migration Job template**

```yaml
# deploy/helm/notebook/templates/migration-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "notebook.fullname" . }}-migrate-{{ .Release.Revision }}
  labels:
    {{- include "notebook.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": pre-upgrade,pre-install
    "helm.sh/hook-weight": "0"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 2
  template:
    metadata:
      labels:
        {{- include "notebook.selectorLabels" . | nindent 8 }}
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          args: ["--migrate-only"]
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: {{ required "postgresql.credentialsSecretRef.name is required" .Values.postgresql.credentialsSecretRef.name }}
                  key: {{ .Values.postgresql.credentialsSecretRef.key }}
            - name: OIDC_ISSUER_URL
              value: {{ required "oidc.issuerURL is required" .Values.oidc.issuerURL | quote }}
            - name: OIDC_REDIRECT_URL
              value: {{ required "oidc.redirectURL is required" .Values.oidc.redirectURL | quote }}
            - name: OIDC_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: {{ required "oidc.clientIDSecretRef.name is required" .Values.oidc.clientIDSecretRef.name }}
                  key: {{ .Values.oidc.clientIDSecretRef.key }}
            - name: OIDC_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: {{ required "oidc.clientSecretSecretRef.name is required" .Values.oidc.clientSecretSecretRef.name }}
                  key: {{ .Values.oidc.clientSecretSecretRef.key }}
            - name: SESSION_HASH_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ required "session.hashKeySecretRef.name is required" .Values.session.hashKeySecretRef.name }}
                  key: {{ .Values.session.hashKeySecretRef.key }}
            - name: SESSION_BLOCK_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ required "session.blockKeySecretRef.name is required" .Values.session.blockKeySecretRef.name }}
                  key: {{ .Values.session.blockKeySecretRef.key }}
```

Note: the Job requires the same OIDC/session env vars as the main
Deployment purely because `config.Load()` validates all of them are
present before returning — not because the migration path uses them.
This is a direct consequence of Plan 2's `config.Load` design (it
doesn't offer a "DB-only" config mode); that is fine to leave as-is for
this plan, but flag it as a possible future simplification rather than
silently working around it here.

- [ ] **Step 3: Verify the Job renders and uses a unique name per revision**

Run (reusing the full `--set` list), piping to `-s templates/migration-job.yaml`:
Expected: a `Job` named `<release>-notebook-migrate-1` on first
install; re-running `helm template` with `--set` changes doesn't
change the rendered revision number by itself (Helm increments
`.Release.Revision` on real installs/upgrades, not on `helm template`
against the same values) — this is expected and fine; the behavior
that matters is verified live during an actual `helm upgrade` in Plan
6's e2e suite, which re-installs the chart multiple times.

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/notebook/templates/migration-job.yaml cmd/server/main.go
git commit -m "feat: add pre-upgrade migration Job and --migrate-only server flag"
```

---

### Task 5: Prometheus Operator scrape config and chart test hook

**Files:**
- Create: `deploy/helm/notebook/templates/podmonitor.yaml`
- Create: `deploy/helm/notebook/templates/tests/test-connection.yaml`

**Interfaces:**
- Consumes: `notebook.selectorLabels`/`notebook.fullname` (Task 1), `.Values.service.port` (Task 1).

- [ ] **Step 1: Write the PodMonitor template**

```yaml
# deploy/helm/notebook/templates/podmonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: {{ include "notebook.fullname" . }}
  labels:
    {{- include "notebook.labels" . | nindent 4 }}
spec:
  selector:
    matchLabels:
      {{- include "notebook.selectorLabels" . | nindent 6 }}
  podMetricsEndpoints:
    - port: http
      path: /metrics
```

- [ ] **Step 2: Write a `helm test` connection check**

```yaml
# deploy/helm/notebook/templates/tests/test-connection.yaml
apiVersion: v1
kind: Pod
metadata:
  name: {{ include "notebook.fullname" . }}-test-connection
  labels:
    {{- include "notebook.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": test
spec:
  restartPolicy: Never
  containers:
    - name: curl
      image: curlimages/curl:8.10.1
      args:
        - "-sf"
        - "http://{{ include "notebook.fullname" . }}:{{ .Values.service.port }}/healthz"
```

- [ ] **Step 3: Verify both templates render**

Run the full `helm template` command from Task 2 Step 4 and confirm
`kind: PodMonitor` and the test `Pod` both appear in the output with no
template errors.

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/notebook/templates/podmonitor.yaml deploy/helm/notebook/templates/tests/test-connection.yaml
git commit -m "feat: add Prometheus PodMonitor and helm test connection check"
```

---

### Task 6: Environment values overrides and chart README

**Files:**
- Create: `deploy/helm/notebook/values-dev.yaml`
- Create: `deploy/helm/notebook/values-prod.yaml`
- Create: `deploy/helm/notebook/README.md`

**Interfaces:** none beyond the values schema already established.

- [ ] **Step 1: Write the dev values override**

```yaml
# deploy/helm/notebook/values-dev.yaml
replicaCount: 1

image:
  tag: dev

postgresql:
  instances: 1
  storageSize: 2Gi
```

- [ ] **Step 2: Write the prod values override**

```yaml
# deploy/helm/notebook/values-prod.yaml
replicaCount: 2

postgresql:
  instances: 3
  storageSize: 20Gi
```

- [ ] **Step 3: Write the chart README documenting cluster prerequisites**

```markdown
# notebook Helm chart

Deploys the internal notebook app: a Go/Vue web app backed by
PostgreSQL via CloudNativePG.

## Cluster prerequisites (not installed by this chart)

This chart assumes the following are already installed on the target
cluster:

- **cert-manager**, with a `ClusterIssuer` matching whatever you pass
  as `ingress.tls.clusterIssuer`.
- **An ingress controller**, with an `IngressClass` matching whatever
  you pass as `ingress.className`.
- **The CloudNativePG operator**, so this chart's `Cluster` custom
  resource (`postgresql.cnpg.io/v1`) is reconciled into a running
  Postgres instance.
- **The Prometheus Operator**, so this chart's `PodMonitor` resource is
  picked up by your cluster's Prometheus instance.

This chart does not install any of the above and will fail at apply
time (for the CRDs it uses) if they're missing.

## Required secrets

This chart never creates Kubernetes `Secret` objects — create them
however your organization already manages secrets, then point the
chart at them via these values:

- `postgresql.credentialsSecretRef`: a secret with a key holding a full
  `postgres://` DSN, e.g. the connection string CloudNativePG generates
  for the `Cluster` this chart creates (CNPG auto-generates an
  `app`-user secret named `<cluster-name>-app`; point this value at
  that secret's `uri` key, or your own if you manage credentials
  differently).
- `oidc.clientIDSecretRef`, `oidc.clientSecretSecretRef`: your OIDC
  application's client ID and secret.
- `session.hashKeySecretRef`, `session.blockKeySecretRef`: two
  independently generated 32-byte random values (e.g.
  `openssl rand -hex 32`) used to encrypt the app's session cookie.

## Installing

```bash
helm install notebook deploy/helm/notebook \
  -f deploy/helm/notebook/values-prod.yaml \
  --set ingress.className=nginx \
  --set ingress.host=notebook.example.com \
  --set ingress.tls.clusterIssuer=letsencrypt-prod \
  --set postgresql.credentialsSecretRef.name=notebook-postgres-app \
  --set oidc.issuerURL=https://idp.example.com \
  --set oidc.redirectURL=https://notebook.example.com/auth/callback \
  --set oidc.clientIDSecretRef.name=notebook-oidc \
  --set oidc.clientSecretSecretRef.name=notebook-oidc \
  --set session.hashKeySecretRef.name=notebook-session \
  --set session.blockKeySecretRef.name=notebook-session
```
```

- [ ] **Step 4: Run `helm lint` against all three values files**

Run each of:
```bash
helm lint deploy/helm/notebook --set ingress.className=nginx --set ingress.host=notebook.example.com --set ingress.tls.clusterIssuer=letsencrypt-prod --set postgresql.credentialsSecretRef.name=x --set oidc.issuerURL=https://idp.example.com --set oidc.redirectURL=https://x/auth/callback --set oidc.clientIDSecretRef.name=x --set oidc.clientSecretSecretRef.name=x --set session.hashKeySecretRef.name=x --set session.blockKeySecretRef.name=x
helm lint deploy/helm/notebook -f deploy/helm/notebook/values-dev.yaml --set ingress.className=nginx --set ingress.host=notebook.example.com --set ingress.tls.clusterIssuer=letsencrypt-prod --set postgresql.credentialsSecretRef.name=x --set oidc.issuerURL=https://idp.example.com --set oidc.redirectURL=https://x/auth/callback --set oidc.clientIDSecretRef.name=x --set oidc.clientSecretSecretRef.name=x --set session.hashKeySecretRef.name=x --set session.blockKeySecretRef.name=x
helm lint deploy/helm/notebook -f deploy/helm/notebook/values-prod.yaml --set ingress.className=nginx --set ingress.host=notebook.example.com --set ingress.tls.clusterIssuer=letsencrypt-prod --set postgresql.credentialsSecretRef.name=x --set oidc.issuerURL=https://idp.example.com --set oidc.redirectURL=https://x/auth/callback --set oidc.clientIDSecretRef.name=x --set oidc.clientSecretSecretRef.name=x --set session.hashKeySecretRef.name=x --set session.blockKeySecretRef.name=x
```
Expected: `0 chart(s) failed` for all three.

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/notebook/values-dev.yaml deploy/helm/notebook/values-prod.yaml deploy/helm/notebook/README.md
git commit -m "docs: add dev/prod values overrides and chart prerequisites README"
```

---

## Out of scope for this plan (covered by later plans)

- Actually installing cert-manager/ingress-controller/CNPG
  operator/Prometheus Operator anywhere (documented prerequisite only).
- CI wiring to run `helm lint`/`helm template` automatically (Plan 6).
- Proving the chart actually deploys a working app end-to-end against
  a real cluster (Plan 6's kind-based e2e suite is what exercises this
  chart for real, including installing it against a real CNPG operator
  and a real Dex OIDC provider).

## Handoff to Plan 6

Plan 6 (`docs/superpowers/plans/2026-10-19-notebook-e2e-ci.md`) assumes
this chart lives at `deploy/helm/notebook`, that its required values
are exactly the ones named in this plan's `README.md` "Installing"
example, that the migration Job is named
`<fullname>-migrate-<revision>` and is a `pre-upgrade,pre-install`
Helm hook, and that `--migrate-only` is the exact flag name added to
`cmd/server/main.go`. Once this plan's tasks are complete and its final
whole-branch review has passed, **the reviewer who signed off on that
review must read Plan 6 against what was actually built** (chart path,
required value names, hook annotations, flag name) and correct any
drift before Plan 6 execution begins.
