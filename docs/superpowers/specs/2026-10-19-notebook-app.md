# Notebook Web App — Spec

Settled design from the grilling session on 2026-10-19. This is the
reference document for all implementation plans in
`docs/superpowers/plans/`. Where a plan and this spec disagree, fix the
plan — this file is the source of truth for product and architecture
decisions.

## Scope

Internal tool for a small team (tens to low hundreds of users, hundreds of
notes). Not designed for org-wide scale, multi-tenancy, or high
availability by default.

## Domain model

- **Note**: `id`, `title`, `content` (Markdown), `author_id` (FK → users),
  `created_at`, `updated_at`, `deleted_at` (nullable — soft delete).
- **Comment**: `id`, `note_id` (FK → notes), `content` (Markdown),
  `author_id` (FK → users), `created_at`, `updated_at`, `deleted_at`
  (nullable). Flat list, no threading (no `parent_comment_id`).
- **User**: `id`, `oidc_subject` (unique, the OIDC `sub` claim), `email`,
  `display_name`, `role` (`user` | `admin`), `created_at`, `updated_at`.
  Upserted from OIDC claims on every successful login.

Authorization rules:
- All notes are visible to all authenticated users (no private/ACL notes).
- A note's author or an admin may edit or soft-delete it.
- Soft-deleting a note cascades a soft-delete to all its comments.
- A comment's author or an admin may soft-delete that single comment,
  independent of its note.
- Soft-deleted rows are hidden from all reads except explicit admin
  audit views (not in v1 scope beyond the flag existing).
- No version history beyond `created_at`/`updated_at`.
- "Author" is not a stored role — it is the ownership relationship
  carried by `author_id`. The only stored roles are `user` and `admin`,
  derived from a configurable OIDC claim (e.g. `groups` contains
  `notebook-admin`) at login/token-refresh time.

Note list (main view):
- Sorted by `updated_at` descending by default.
- Filterable by a text search (title, and content via Postgres
  `ILIKE`/`tsvector`) and by author.
- Offset pagination (no infinite scroll).

Content handling:
- Markdown only — no file/image attachments, no object storage
  dependency for content.
- Rendered server-side with `goldmark`, sanitized with `bluemonday`
  before being sent to the client.
- The note/comment editor has write and preview tabs (GitHub-style). The
  write tab is a plain `<textarea>`. The preview tab calls the backend's
  render endpoint so preview output exactly matches stored/displayed
  output (no client-side Markdown dialect drift).

## Authentication

- OIDC-generic: any standard OIDC issuer, configured via Helm values
  (issuer URL, client ID, client secret reference). No bundled IdP.
- **BFF pattern**: the Go backend is the OIDC confidential client,
  performs the Authorization Code + PKCE flow server-side, and issues its
  own HttpOnly session cookie to the Vue SPA. The SPA never holds the
  OIDC token.
- Role (`admin` vs `user`) is derived from a configurable claim name/value
  on the ID token or userinfo response, re-evaluated on login.

## Backend

- Language/runtime: Go.
- Router: `gorilla/mux` (team's explicit choice; note upstream project is
  in maintenance-only mode — acceptable for this scope, revisit if it
  becomes a blocker).
- Migrations: `goose`, migration files embedded in the binary via
  `embed.FS`, applied by a Helm pre-upgrade hook Job before new pods
  start.
- Logging: structured JSON via `log/slog`. No distributed tracing (single
  service, no multi-hop request path to justify it).
- Metrics: Prometheus via `prometheus/client_golang`, exposed on a
  `/metrics` endpoint.
  - HTTP server metrics: request count, duration histogram, in-flight
    gauge — labeled by route and status code.
  - Go runtime metrics (default collector).
  - Business counters: `notes_created_total`, `comments_created_total`,
    `comments_deleted_total`.
- Static assets: the built Vue app is embedded into the Go binary via
  `embed.FS` and served directly — one container image, one pod, no
  separate nginx/static-file pod.

## Frontend

- Vue 3 (Composition API), Vite, Pinia (state), vue-router, Tailwind CSS.
- Main view: a list pane (notes) + detail pane (selected note's content,
  rendered Markdown, and its comments).
- Editor: write/preview tabs as described above.
- Comments: flat list under a note's content; author can edit/delete
  their own; admin can delete any.

## Database

- PostgreSQL, provisioned via a CloudNativePG `Cluster` custom resource
  created by the Helm chart (the chart does **not** install the CNPG
  operator itself — that's a documented cluster prerequisite).
- Default `values.yaml`: 1 instance (no HA), 5Gi storage. HA achievable
  via a documented `postgresql.instances: 3` override.
- Backup (WAL archiving + Barman base backups to object storage) is
  optional, off by default, with a documented values block for pointing
  at an S3-compatible bucket.

## Deployment / Kubernetes / Helm

- Monorepo: backend, frontend, and `deploy/helm/<chart-name>` live
  together in one repo.
- Single multi-stage Dockerfile: Node stage builds the Vue app → Go stage
  embeds the built assets and compiles the binary → final runtime stage
  is `gcr.io/distroless/static`.
- Helm chart assumes these are pre-installed cluster prerequisites
  (documented in the chart's README, not installed by the chart):
  cert-manager, an ingress controller, the CloudNativePG operator, the
  Prometheus Operator.
- Secrets (OIDC client secret, DB credentials) are referenced by
  name/key via Helm values (e.g. `oidcClientSecretRef: {name, key}`) —
  the chart does not create Secret objects itself, staying agnostic to
  how the deploying org manages secrets.
- Ingress class name and cert-manager ClusterIssuer name are required,
  empty-by-default values — no hardcoded assumption about either.
- Single backend replica by default (`replicaCount` value), no
  autoscaling (HPA) in v1.
- One chart, multiple values files: `values.yaml` (defaults),
  `values-dev.yaml`, `values-prod.yaml` (overrides).

## CI/CD

- GitHub Actions: build/push the container image, run `helm lint` and
  `helm template` on every PR.
- Deployment itself is manual `helm upgrade --install` for v1 — no
  GitOps operator assumed.

## Testing

Two end-to-end suites, modeled on the pattern used in
[timadorus/platform](https://github.com/timadorus/platform), both run in
GitHub Actions CI on every PR:

1. **Infra/cluster e2e**: a small Go `devcluster`-style CLI that, in
   strict order, (a) ensures a throwaway **kind** cluster, (b) installs
   cert-manager, (c) installs the Prometheus Operator, (d) installs the
   CloudNativePG operator, (e) installs an ingress controller, (f)
   bootstraps **Dex** as the test OIDC provider (static test users/
   passwords — Dex chosen over Zitadel/Keycloak for this role because it
   needs far less bootstrap code for a throwaway test IdP), (g) creates
   the CNPG `Cluster` CR, (h) builds/loads the app's container image into
   kind, (i) installs the app via the **real** production Helm chart,
   (j) port-forwards to the app. Assertions via **Ginkgo v2 / Gomega**,
   gated behind a `//go:build e2e` tag, run with
   `go test -tags e2e ./test/e2e/... -timeout 30m`. Teardown runs in
   strict reverse order, keeping the CNPG operator alive until after the
   app namespace (and its Cluster CR) is deleted.
2. **Frontend e2e**: **Playwright**, driving the real built frontend
   against a running backend (either the kind environment from suite 1,
   or a lighter docker-compose stack — decided in the frontend/e2e plan).

Unit and integration tests (e.g. repository-layer tests against a real
Postgres via `testcontainers-go`) are written test-first per task in each
plan; they are not a separate phase.

## Explicitly out of scope for v1

- Note versioning/edit history.
- Private notes / per-note sharing or ACLs.
- File/image attachments, object storage.
- Real-time updates (WebSocket/SSE) — manual refresh/poll only.
- Distributed tracing.
- Autoscaling (HPA).
- GitOps-based deployment.
- Multiple IdP bundling — OIDC issuer is always externally supplied.
