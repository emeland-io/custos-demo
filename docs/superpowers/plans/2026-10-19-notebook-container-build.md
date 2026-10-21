# Notebook App — Container Build & Static Asset Embedding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed the built Vue frontend into the Go binary via `embed.FS`,
serve it with SPA-aware fallback routing alongside the existing API, and
produce a single multi-stage Dockerfile (Node build → Go build →
distroless runtime) plus a local, container-free dev workflow for
Postgres.

**Architecture:** `internal/webassets` holds a committed placeholder
`dist/` directory (required by Go's `embed` semantics — the directory
must exist and contain at least one non-hidden file at compile time)
that real frontend builds overwrite. `internal/api` gets a new
`SPAHandler` that serves static files by exact path and falls back to
`index.html` for any unmatched path, so `vue-router`'s HTML5 history
mode works without a separate web server. The Dockerfile's frontend
stage produces `frontend/dist`, which is copied over
`internal/webassets/dist` before `go build` so the real assets — not
the placeholder — end up embedded in the shipped binary.

**Tech Stack:** Go `embed` package, Docker multi-stage build,
`gcr.io/distroless/static:nonroot` runtime image, `docker compose` for
local Postgres-only dev.

**Spec:** `docs/superpowers/specs/2026-10-19-notebook-app.md`

## Handoff Note (read before starting)

This plan assumes Plan 3
(`docs/superpowers/plans/2026-10-19-notebook-frontend.md`) was
implemented exactly as written: `frontend/package.json`'s `build`
script produces `frontend/dist/`, and `api.NewRouter`'s signature from
Plan 2's final state (`logger, notes, comments, auth AuthWiring`) is
what this plan's `NewRouter` signature change applies on top of.
**Before this plan's first task starts, the reviewer who signed off on
Plan 3's final whole-branch review must re-read this plan against the
actual Plan 1–3 codebase** (build output path, exact `NewRouter`
parameter list) and correct any drift before proceeding.

## Global Constraints

- Exactly one container image is produced; there is no separate
  nginx/static-file-serving pod or container.
- The Go binary must build and run correctly even when
  `internal/webassets/dist` contains only the committed placeholder
  (e.g., a bare `go build ./...` in CI before the frontend is built) —
  it just serves the placeholder page instead of the real app.
- Real frontend build output overwrites, but never permanently
  replaces in git, the placeholder: `internal/webassets/dist/*` is
  gitignored except the placeholder file itself.
- The final runtime image runs as a non-root user and contains no
  shell (`distroless/static:nonroot`).
- The SPA fallback handler must never intercept `/api/*`, `/auth/*`,
  `/healthz`, or `/metrics` — those routes are registered before the
  catch-all and take priority.

## Review Focus

- A request for a path that looks like a static asset but doesn't
  exist in the embedded FS (e.g. `/assets/missing.js`) — must 404 (or
  at minimum not silently serve `index.html` as if it were JS, which
  would produce a confusing "Unexpected token '<'" browser error);
  only *navigational* paths (no file extension match) fall back to
  `index.html`.
- A `go build ./...` run with only the placeholder `dist/` present
  (simulating a fresh clone before `npm run build` has ever run) —
  must succeed, not fail with an embed compile error.
- The Dockerfile build with a frontend that fails its build step (e.g.
  a broken `npm run build`) — the overall `docker build` must fail
  loudly at that stage, not silently ship a stale/placeholder frontend
  in a "successful" image.
- Request to `/` with no trailing content — must serve `index.html`,
  not 404.
- `docker-compose.yml`'s Postgres service — must not collide with a
  developer's own local Postgres on the default `5432` port by
  silently failing to start; it should be easy to tell from `docker
  compose up` output if that happens.

---

## File Structure

```
internal/webassets/dist/README.txt      (committed placeholder)
internal/webassets/webassets.go
internal/webassets/webassets_test.go
internal/api/spa_handler.go
internal/api/spa_handler_test.go
Dockerfile
.dockerignore
Makefile
docker-compose.yml
```

---

### Task 1: Embedded asset package with committed placeholder

**Files:**
- Create: `internal/webassets/dist/README.txt`
- Create: `internal/webassets/webassets.go`
- Test: `internal/webassets/webassets_test.go`
- Modify: `.gitignore`

**Interfaces:**
- Produces: `webassets.FS() (fs.FS, error)` — returns the embedded tree rooted so that `FS.Open("index.html")` works (not `FS.Open("dist/index.html")`).

- [ ] **Step 1: Create the committed placeholder**

```text
# internal/webassets/dist/README.txt
This directory is intentionally committed as a placeholder.

Go's `//go:embed all:dist` directive requires this directory to exist
and contain at least one tracked, non-hidden file at compile time, or
`go build` fails for anyone who hasn't built the frontend yet.

Running `make frontend-dist` (or the Docker build) overwrites every
file in this directory, including this one, with the real built Vue
app. Local, uncommitted changes made by a frontend build are ignored
by git (see .gitignore) except for this file, which git always tracks
so a fresh clone keeps building.
```

- [ ] **Step 2: Add the gitignore rule**

Append to `.gitignore`:

```
internal/webassets/dist/*
!internal/webassets/dist/README.txt
```

- [ ] **Step 3: Write the failing test**

```go
// internal/webassets/webassets_test.go
package webassets_test

import (
	"io/fs"
	"testing"

	"notebook/internal/webassets"
)

func TestFS_ContainsIndexAtRoot(t *testing.T) {
	assets, err := webassets.FS()
	if err != nil {
		t.Fatalf("FS() failed: %v", err)
	}

	// With only the committed placeholder present, there is no
	// index.html — this test only asserts the FS is rooted correctly
	// (README.txt is readable at the root, not under "dist/").
	entries, err := fs.ReadDir(assets, ".")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "README.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected README.txt at FS root, got entries: %+v", entries)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/webassets/... -v`
Expected: FAIL — package `webassets` undefined.

- [ ] **Step 5: Implement the embedded FS wrapper**

```go
// internal/webassets/webassets.go
package webassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded frontend build output rooted at "dist", so
// callers use FS.Open("index.html") rather than FS.Open("dist/index.html").
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/webassets/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/webassets .gitignore
git commit -m "feat: add embedded frontend asset package with build placeholder"
```

---

### Task 2: SPA fallback handler

**Files:**
- Create: `internal/api/spa_handler.go`
- Test: `internal/api/spa_handler_test.go`

**Interfaces:**
- Produces: `api.NewSPAHandler(assets fs.FS) http.Handler`.

- [ ] **Step 1: Write the failing test**

```go
// internal/api/spa_handler_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":      {Data: []byte("<html>spa shell</html>")},
		"assets/app.js":   {Data: []byte("console.log('app')")},
		"assets/app.css":  {Data: []byte("body{color:red}")},
	}
}

func TestSPAHandler_ServesExistingAsset(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "console.log('app')" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestSPAHandler_FallsBackToIndexForUnknownNavigationalPath(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/notes/42", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>spa shell</html>" {
		t.Fatalf("expected index.html fallback, got: %s", rec.Body.String())
	}
}

func TestSPAHandler_ServesIndexAtRoot(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "<html>spa shell</html>" {
		t.Fatalf("expected index.html at root, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_MissingAssetLikePath_404sInsteadOfFallback(t *testing.T) {
	handler := NewSPAHandler(testAssets())

	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing asset path, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestSPAHandler -v`
Expected: FAIL — `NewSPAHandler` undefined.

- [ ] **Step 3: Implement the SPA handler**

```go
// internal/api/spa_handler.go
package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// NewSPAHandler serves files from assets by exact path. A request
// whose path doesn't exist in assets and looks like a navigational
// route (no file extension) falls back to index.html, so vue-router's
// HTML5 history mode works without a separate web server. A request
// for a path with a file extension (e.g. "/assets/missing.js") that
// doesn't exist 404s instead, so broken asset references are visible
// as 404s in the browser's network tab rather than masquerading as
// index.html and failing with a confusing script-parse error.
func NewSPAHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleaned := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleaned == "" || cleaned == "." {
			cleaned = "index.html"
		}

		if _, err := fs.Stat(assets, cleaned); err != nil {
			if path.Ext(cleaned) != "" {
				http.NotFound(w, r)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestSPAHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/spa_handler.go internal/api/spa_handler_test.go
git commit -m "feat: add SPA fallback handler for embedded frontend assets"
```

---

### Task 3: Wire the SPA handler into the router and main.go

**Files:**
- Modify: `internal/api/router.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `webassets.FS` (Task 1), `NewSPAHandler` (Task 2).
- Produces: `api.NewRouter` gains an `assets fs.FS` parameter; the catch-all SPA route is registered last so it never shadows `/api`, `/auth`, `/healthz`, or `/metrics`.

- [ ] **Step 1: Update `NewRouter`'s signature and add the catch-all route**

```go
// internal/api/router.go
package api

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/metrics"
	"notebook/internal/repo"
)

func NewRouter(logger *slog.Logger, notes *repo.NoteRepo, comments *repo.CommentRepo, auth AuthWiring, assets fs.FS) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)
	r.Handle("/metrics", metrics.Handler()).Methods(http.MethodGet)

	r.HandleFunc("/auth/login", auth.LoginHandler).Methods(http.MethodGet)
	r.HandleFunc("/auth/callback", auth.CallbackHandler).Methods(http.MethodGet)
	r.HandleFunc("/auth/logout", auth.LogoutHandler).Methods(http.MethodPost)

	api := r.PathPrefix("/api").Subrouter()
	api.Use(auth.SessionMiddleware)
	api.HandleFunc("/me", MeHandler).Methods(http.MethodGet)
	api.HandleFunc("/render-preview", RenderPreviewHandler).Methods(http.MethodPost)

	notesHandler := NewNotesHandler(notes)
	api.HandleFunc("/notes", notesHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/notes", notesHandler.Create).Methods(http.MethodPost)
	api.HandleFunc("/notes/{id}", notesHandler.Get).Methods(http.MethodGet)
	api.HandleFunc("/notes/{id}", notesHandler.Update).Methods(http.MethodPut)
	api.HandleFunc("/notes/{id}", notesHandler.Delete).Methods(http.MethodDelete)

	commentsHandler := NewCommentsHandler(comments)
	api.HandleFunc("/notes/{noteId}/comments", commentsHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/notes/{noteId}/comments", commentsHandler.Create).Methods(http.MethodPost)
	api.HandleFunc("/comments/{id}", commentsHandler.Update).Methods(http.MethodPut)
	api.HandleFunc("/comments/{id}", commentsHandler.Delete).Methods(http.MethodDelete)

	// Catch-all for the SPA — registered last so it never shadows the
	// routes above; mux matches routes in registration order.
	r.PathPrefix("/").Handler(NewSPAHandler(assets))

	return metrics.Middleware(r)
}
```

- [ ] **Step 2: Wire the embedded assets into `main.go`**

```go
// cmd/server/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"

	"notebook/internal/api"
	"notebook/internal/auth"
	"notebook/internal/config"
	"notebook/internal/db"
	"notebook/internal/db/migrations"
	"notebook/internal/logging"
	"notebook/internal/repo"
	"notebook/internal/webassets"
)

func main() {
	logger := logging.New()
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", slog.String("error", err.Error()))
		return
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		return
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		logger.Error("failed to run migrations", slog.String("error", err.Error()))
		return
	}

	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	provider, err := auth.NewProvider(ctx, cfg.OIDC)
	if err != nil {
		logger.Error("failed to initialize OIDC provider", slog.String("error", err.Error()))
		return
	}
	flowCodec := auth.NewFlowCodec(cfg.Session.HashKey, cfg.Session.BlockKey)
	sessionCodec := auth.NewSessionCodec(cfg.Session.HashKey, cfg.Session.BlockKey)

	assets, err := webassets.FS()
	if err != nil {
		logger.Error("failed to load embedded frontend assets", slog.String("error", err.Error()))
		return
	}

	router := api.NewRouter(logger, notes, comments, api.AuthWiring{
		SessionMiddleware: auth.NewSessionMiddleware(sessionCodec, users),
		LoginHandler:      auth.NewLoginHandler(provider, flowCodec),
		CallbackHandler:   auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, cfg.OIDC.AdminClaim, cfg.OIDC.AdminClaimValue, "/"),
		LogoutHandler:     auth.NewLogoutHandler(sessionCodec),
	}, assets)

	logger.Info("starting server", slog.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", router); err != nil {
		logger.Error("server stopped", slog.String("error", err.Error()))
	}
}
```

- [ ] **Step 3: Run the full backend test suite to verify nothing regressed**

Run: `go test ./... -v`
Expected: PASS (all packages)

- [ ] **Step 4: Commit**

```bash
git add internal/api/router.go cmd/server/main.go
git commit -m "feat: serve embedded frontend assets with SPA fallback from the main router"
```

---

### Task 4: Multi-stage Dockerfile and local frontend-embedding Makefile target

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `Makefile`

**Interfaces:**
- Produces: `docker build -t notebook:dev .` succeeds and runs; `make frontend-dist` copies a real `frontend/dist` build over `internal/webassets/dist` for local (non-Docker) runs.

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS backend-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN rm -rf internal/webassets/dist && mkdir -p internal/webassets/dist
COPY --from=frontend-build /src/frontend/dist/ internal/webassets/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/notebook-server ./cmd/server

FROM gcr.io/distroless/static:nonroot AS runtime
COPY --from=backend-build /out/notebook-server /notebook-server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/notebook-server"]
```

- [ ] **Step 2: Write `.dockerignore`**

```text
.git
docs
frontend/node_modules
frontend/dist
internal/webassets/dist/*
!internal/webassets/dist/README.txt
```

- [ ] **Step 3: Write the Makefile**

```makefile
.PHONY: frontend-dist test build docker-build

frontend-dist:
	cd frontend && npm ci && npm run build
	rm -rf internal/webassets/dist
	mkdir -p internal/webassets/dist
	cp -r frontend/dist/. internal/webassets/dist/

test:
	go test ./...
	cd frontend && npm run test

build: frontend-dist
	go build -o bin/notebook-server ./cmd/server

docker-build:
	docker build -t notebook:dev .
```

- [ ] **Step 4: Build the image to verify the Dockerfile is correct**

Run: `docker build -t notebook:dev .`
Expected: all three stages complete without error, producing a
`notebook:dev` image.

- [ ] **Step 5: Smoke-test the image's health endpoint**

Run (requires a reachable Postgres and dummy OIDC/session env vars —
the container will fail to start without them per `config.Load`'s
validation from Plan 2, which is expected; this step only confirms the
binary itself starts enough to read its config and attempt a DB
connection, not a full boot):

```bash
docker run --rm -e DATABASE_URL=postgres://invalid -e OIDC_ISSUER_URL=https://example.com -e OIDC_CLIENT_ID=x -e OIDC_CLIENT_SECRET=x -e OIDC_REDIRECT_URL=https://example.com/auth/callback -e SESSION_HASH_KEY=0123456789abcdef0123456789abcdef -e SESSION_BLOCK_KEY=0123456789abcdef0123456789abcdef notebook:dev
```

Expected: a structured JSON log line about failing to connect to the
database (proving the binary runs, parses config, and reaches the DB
connection step) rather than a crash on startup before that point.

- [ ] **Step 6: Commit**

```bash
git add Dockerfile .dockerignore Makefile
git commit -m "feat: add multi-stage Dockerfile and local build Makefile targets"
```

---

### Task 5: Local Postgres-only dev stack via docker compose

**Files:**
- Create: `docker-compose.yml`
- Modify: `README.md`

**Interfaces:**
- Produces: `docker compose up -d postgres` starts a local Postgres the Go server (run via `go run ./cmd/server` or `make build && ./bin/notebook-server`) can connect to with `DATABASE_URL`.

- [ ] **Step 1: Write `docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: notebook
      POSTGRES_USER: notebook
      POSTGRES_PASSWORD: notebook
    ports:
      - "5432:5432"
    volumes:
      - notebook-postgres-data:/var/lib/postgresql/data

volumes:
  notebook-postgres-data:
```

- [ ] **Step 2: Document the local dev workflow in the README**

Append a section to `README.md`:

```markdown
## Local development

1. Start Postgres: `docker compose up -d postgres`. If port 5432 is
   already in use by another local Postgres, `docker compose up`
   reports the bind failure — stop the other instance or change the
   published port in `docker-compose.yml`.
2. Build the frontend into the Go binary's embedded assets:
   `make frontend-dist`.
3. Set the required environment variables (see
   `internal/config/config.go` for the full list): `DATABASE_URL`,
   `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`,
   `OIDC_REDIRECT_URL`, `SESSION_HASH_KEY`, `SESSION_BLOCK_KEY`. A real
   OIDC provider is required for the login flow to complete locally —
   Plan 6's Dex-based e2e environment is the quickest way to get one
   running without registering with an external IdP.
4. Run the server: `go run ./cmd/server`.
5. Run all tests: `make test` (Go repository tests spin up their own
   throwaway Postgres containers via testcontainers-go — they do not
   use the `docker-compose.yml` Postgres instance).
```

- [ ] **Step 3: Verify the compose stack starts**

Run: `docker compose up -d postgres && docker compose ps`
Expected: the `postgres` service shows as running/healthy.

Run: `docker compose down -v` to clean up afterward.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml README.md
git commit -m "docs: add local Postgres dev stack and development workflow"
```

---

## Out of scope for this plan (covered by later plans)

- Kubernetes manifests / Helm chart (Plan 5) — this plan only produces
  the image the chart will reference.
- CI wiring to actually build and push this image (Plan 6).
- A real OIDC provider for local/dev use (Plan 6's Dex bootstrap is the
  first point in this project where one exists outside a developer's
  own IdP).

## Handoff to Plan 5

Plan 5 (`docs/superpowers/plans/2026-10-19-notebook-helm-chart.md`)
assumes this plan's image exposes port `8080`, serves `/healthz` for
liveness/readiness probes, serves `/metrics` for Prometheus scraping,
and reads all configuration exclusively from the environment variables
named in `internal/config/config.go` (no config file, no command-line
flags). Once this plan's tasks are complete and its final
whole-branch review has passed, **the reviewer who signed off on that
review must read Plan 5 against what was actually built** (exact env
var names, port number, probe paths) and correct any drift before Plan
5 execution begins.
