# Notebook App — Backend Domain & API Foundations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Go backend's domain model, Postgres-backed repositories,
and HTTP API for notes and comments (list/create/edit/soft-delete,
Markdown rendering, Prometheus metrics), with a stubbed auth context that
Plan 2 (OIDC/BFF) will replace without changing any handler signatures.

**Architecture:** A layered Go service: `internal/models` (plain structs),
`internal/repo` (pgx-backed repositories, one per entity, tested against a
real Postgres via testcontainers-go), `internal/api` (gorilla/mux handlers
+ middleware), `internal/markdown` (goldmark+bluemonday rendering),
`internal/metrics` (Prometheus collectors). Auth is abstracted behind a
`CurrentUser(ctx) (*models.User, bool)` accessor so this plan can ship
with a stub middleware and Plan 2 can swap in the real OIDC/BFF session
middleware later with no handler changes.

**Tech Stack:** Go 1.23+, `gorilla/mux`, `jackc/pgx/v5`, `pressly/goose/v3`,
`yuin/goldmark`, `microcosm-cc/bluemonday`, `prometheus/client_golang`,
`testcontainers-go` (+ postgres module) for repo tests, `log/slog`.

**Spec:** `docs/superpowers/specs/2026-10-19-notebook-app.md`

## Global Constraints

- Go module path: `notebook` (monorepo root `go.mod`).
- All DB access goes through `pgx/v5`, no ORM.
- Every repository method takes `ctx context.Context` as its first
  argument.
- Soft delete only: no repository method issues a hard `DELETE`.
- All timestamps are `time.Time` in UTC, stored as `timestamptz`.
- Migrations live in `internal/db/migrations/*.sql`, embedded via
  `embed.FS`, applied with `goose`.
- HTTP handlers return JSON; errors use a single `{"error": "..."}`
  envelope with the correct HTTP status — no stack traces or raw Go
  error strings ever reach the client.
- Role/ownership checks happen in handlers via two helpers:
  `isOwnerOrAdmin(user *models.User, authorID int64) bool` and
  `isAdmin(user *models.User) bool` (both in `internal/api/authz.go`).

## Review Focus

- Empty or whitespace-only note title/content on create/update — a
  reasonable person expects a 400, not a stored blank note.
- A non-author, non-admin user attempting to edit or delete someone
  else's note or comment — must get 403, not a silent no-op or 500.
- Deleting a note that has comments — comments must become invisible to
  subsequent list/get calls, not orphaned or left visible.
- Pagination/filter parameters that are malformed (negative page,
  non-numeric `author_id`, absurdly large `limit`) — must clamp or 400,
  never panic or return unbounded rows.
- Markdown content containing a `<script>` tag or `onerror=` attribute —
  rendered output must have it stripped, both from the stored-note
  render path and the live preview endpoint.

---

## File Structure

```
go.mod
cmd/server/main.go
internal/logging/logger.go
internal/db/db.go
internal/db/migrations/00001_init.sql
internal/db/migrations/migrations.go
internal/models/user.go
internal/models/note.go
internal/models/comment.go
internal/markdown/render.go
internal/repo/user_repo.go
internal/repo/note_repo.go
internal/repo/comment_repo.go
internal/repo/testutil_test.go
internal/metrics/metrics.go
internal/api/router.go
internal/api/authctx.go
internal/api/authz.go
internal/api/notes_handler.go
internal/api/comments_handler.go
internal/api/render_handler.go
internal/api/health_handler.go
```

---

### Task 1: Project scaffold, logging, health endpoint

**Files:**
- Create: `go.mod`
- Create: `internal/logging/logger.go`
- Create: `internal/api/health_handler.go`
- Create: `internal/api/router.go`
- Create: `cmd/server/main.go`
- Test: `internal/api/health_handler_test.go`

**Interfaces:**
- Produces: `logging.New() *slog.Logger`; `api.NewRouter(logger *slog.Logger) *mux.Router`; `api.HealthHandler(w http.ResponseWriter, r *http.Request)`.

- [ ] **Step 1: Initialize the Go module and add first dependency**

```bash
go mod init notebook
go get github.com/gorilla/mux@v1.8.1
```

- [ ] **Step 2: Write the failing test for the health handler**

```go
// internal/api/health_handler_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler_ReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestHealthHandler_ReturnsOK -v`
Expected: FAIL — `HealthHandler` undefined.

- [ ] **Step 4: Implement logger, health handler, and router**

```go
// internal/logging/logger.go
package logging

import (
	"log/slog"
	"os"
)

func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
```

```go
// internal/api/health_handler.go
package api

import (
	"net/http"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
```

```go
// internal/api/router.go
package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
)

func NewRouter(logger *slog.Logger) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)
	return r
}
```

```go
// cmd/server/main.go
package main

import (
	"log/slog"
	"net/http"

	"notebook/internal/api"
	"notebook/internal/logging"
)

func main() {
	logger := logging.New()
	router := api.NewRouter(logger)

	logger.Info("starting server", slog.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", router); err != nil {
		logger.Error("server stopped", slog.String("error", err.Error()))
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/... -run TestHealthHandler_ReturnsOK -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/logging internal/api/health_handler.go internal/api/health_handler_test.go internal/api/router.go cmd/server/main.go
git commit -m "feat: scaffold Go module with health endpoint"
```

---

### Task 2: Postgres connection and initial schema migration

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/migrations/00001_init.sql`
- Create: `internal/db/migrations/migrations.go`
- Test: `internal/db/migrations_test.go`

**Interfaces:**
- Consumes: none (first DB-touching task).
- Produces: `db.Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error)`; `migrations.FS embed.FS`; `migrations.Up(ctx context.Context, pool *pgxpool.Pool) error`.

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/jackc/pgx/v5@v5.6.0
go get github.com/pressly/goose/v3@v3.21.1
go get github.com/testcontainers/testcontainers-go@v0.33.0
go get github.com/testcontainers/testcontainers-go/modules/postgres@v0.33.0
```

- [ ] **Step 2: Write the migration SQL**

```sql
-- internal/db/migrations/00001_init.sql
-- +goose Up
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    oidc_subject  TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notes (
    id          BIGSERIAL PRIMARY KEY,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    author_id   BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_notes_updated_at ON notes (updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_notes_author_id ON notes (author_id) WHERE deleted_at IS NULL;

CREATE TABLE comments (
    id          BIGSERIAL PRIMARY KEY,
    note_id     BIGINT NOT NULL REFERENCES notes(id),
    content     TEXT NOT NULL,
    author_id   BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_comments_note_id ON comments (note_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE comments;
DROP TABLE notes;
DROP TABLE users;
```

- [ ] **Step 3: Implement the embedded migrations runner and pool connector**

```go
// internal/db/migrations/migrations.go
package migrations

import (
	"context"
	"embed"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

func Up(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(FS)
	db, err := goose.OpenDBWithDriver("pgx", pool.Config().ConnString())
	if err != nil {
		return err
	}
	defer db.Close()
	return goose.UpContext(ctx, db, ".")
}
```

```go
// internal/db/db.go
package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
```

- [ ] **Step 4: Write a testcontainers-backed test that migrations apply cleanly**

```go
// internal/db/migrations_test.go
package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tclog "github.com/testcontainers/testcontainers-go/log"

	"notebook/internal/db"
	"notebook/internal/db/migrations"
)

func TestMigrations_ApplyCleanly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("notebook"),
		postgres.WithUsername("notebook"),
		postgres.WithPassword("notebook"),
		postgres.WithSQLDriver("pgx"),
		testcontainers_WithLogger(t),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name = 'notes'").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected notes table to exist, got count=%d", count)
	}
}

func testcontainers_WithLogger(t *testing.T) postgres.PostgresContainerOption {
	return postgres.WithSQLDriver("pgx")
}
```

(The repeated `WithSQLDriver` call in the helper is intentionally a
no-op placeholder removed in Task 3's `testutil_test.go`, which Task 2's
test does not yet depend on — ignore `tclog` import if unused and drop
it; Go will fail the build on an unused import, so delete that import
line and the helper function body line referencing it before running.)

- [ ] **Step 5: Run test to verify it fails, then fix import issues and rerun**

Run: `go test ./internal/db/... -run TestMigrations_ApplyCleanly -v`

Remove the unused `tclog` import and the redundant helper function
(replace its single call site with nothing — just drop the
`testcontainers_WithLogger(t)` argument from `postgres.Run`), then rerun.

Expected after cleanup: PASS (container starts, migration creates
`notes` table).

- [ ] **Step 6: Commit**

```bash
git add internal/db
git commit -m "feat: add postgres connection and initial schema migration"
```

---

### Task 3: Shared repository test helper and User model/repository

**Files:**
- Create: `internal/repo/testutil_test.go`
- Create: `internal/models/user.go`
- Create: `internal/repo/user_repo.go`
- Test: `internal/repo/user_repo_test.go`

**Interfaces:**
- Consumes: `db.Connect`, `migrations.Up` (Task 2).
- Produces: `models.User{ID, OIDCSubject, Email, DisplayName, Role, CreatedAt, UpdatedAt}`; `models.RoleUser`, `models.RoleAdmin` constants; `repo.UserRepo` struct with `UpsertByOIDCSubject(ctx, subject, email, displayName, role string) (*models.User, error)` and `GetByID(ctx, id int64) (*models.User, error)`.

- [ ] **Step 1: Write the shared testcontainers helper**

```go
// internal/repo/testutil_test.go
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"notebook/internal/db"
	"notebook/internal/db/migrations"
)

// newTestPool starts a throwaway Postgres container, applies migrations,
// and returns a connected pool. The container is terminated when the
// test completes.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("notebook"),
		postgres.WithUsername("notebook"),
		postgres.WithPassword("notebook"),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	return pool
}
```

- [ ] **Step 2: Write the failing User model and repository test**

```go
// internal/repo/user_repo_test.go
package repo_test

import (
	"context"
	"testing"

	"notebook/internal/models"
	"notebook/internal/repo"
)

func TestUserRepo_UpsertByOIDCSubject_CreatesThenUpdates(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)

	created, err := users.UpsertByOIDCSubject(ctx, "sub-123", "a@example.com", "Alice", models.RoleUser)
	if err != nil {
		t.Fatalf("create upsert failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected assigned ID, got 0")
	}
	if created.Role != models.RoleUser {
		t.Fatalf("expected role %q, got %q", models.RoleUser, created.Role)
	}

	updated, err := users.UpsertByOIDCSubject(ctx, "sub-123", "a@example.com", "Alice Updated", models.RoleAdmin)
	if err != nil {
		t.Fatalf("update upsert failed: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("expected same ID on upsert, got %d vs %d", updated.ID, created.ID)
	}
	if updated.DisplayName != "Alice Updated" || updated.Role != models.RoleAdmin {
		t.Fatalf("expected updated display name/role, got %+v", updated)
	}

	fetched, err := users.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
	if fetched.Email != "a@example.com" {
		t.Fatalf("unexpected email: %s", fetched.Email)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/repo/... -run TestUserRepo_UpsertByOIDCSubject_CreatesThenUpdates -v`
Expected: FAIL — package `repo` / `models` pieces undefined.

- [ ] **Step 4: Implement the User model and repository**

```go
// internal/models/user.go
package models

import "time"

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type User struct {
	ID          int64
	OIDCSubject string
	Email       string
	DisplayName string
	Role        Role
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
```

```go
// internal/repo/user_repo.go
package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"notebook/internal/models"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

func (r *UserRepo) UpsertByOIDCSubject(ctx context.Context, subject, email, displayName string, role models.Role) (*models.User, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (oidc_subject, email, display_name, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (oidc_subject) DO UPDATE
		SET email = EXCLUDED.email,
		    display_name = EXCLUDED.display_name,
		    role = EXCLUDED.role,
		    updated_at = now()
		RETURNING id, oidc_subject, email, display_name, role, created_at, updated_at
	`, subject, email, displayName, string(role))

	return scanUser(row)
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, oidc_subject, email, display_name, role, created_at, updated_at
		FROM users WHERE id = $1
	`, id)
	return scanUser(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*models.User, error) {
	var u models.User
	var role string
	if err := row.Scan(&u.ID, &u.OIDCSubject, &u.Email, &u.DisplayName, &role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.Role = models.Role(role)
	return &u, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/repo/... -run TestUserRepo_UpsertByOIDCSubject_CreatesThenUpdates -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/repo internal/models/user.go
git commit -m "feat: add User model and repository with OIDC upsert"
```

---

### Task 4: Markdown rendering (goldmark + bluemonday)

**Files:**
- Create: `internal/markdown/render.go`
- Test: `internal/markdown/render_test.go`

**Interfaces:**
- Produces: `markdown.Render(source string) (string, error)` — returns sanitized HTML.

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/yuin/goldmark@v1.7.4
go get github.com/microcosm-cc/bluemonday@v1.0.27
```

- [ ] **Step 2: Write the failing tests (happy path + XSS stripping)**

```go
// internal/markdown/render_test.go
package markdown_test

import (
	"strings"
	"testing"

	"notebook/internal/markdown"
)

func TestRender_BasicMarkdown(t *testing.T) {
	html, err := markdown.Render("# Hello\n\nThis is **bold**.")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(html, "<h1>Hello</h1>") {
		t.Fatalf("expected h1 heading, got: %s", html)
	}
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Fatalf("expected strong tag, got: %s", html)
	}
}

func TestRender_StripsScriptTag(t *testing.T) {
	html, err := markdown.Render("hello <script>alert('xss')</script> world")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("expected script tag to be stripped, got: %s", html)
	}
}

func TestRender_StripsOnErrorAttribute(t *testing.T) {
	html, err := markdown.Render(`<img src="x" onerror="alert(1)">`)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if strings.Contains(html, "onerror") {
		t.Fatalf("expected onerror attribute to be stripped, got: %s", html)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/markdown/... -v`
Expected: FAIL — `markdown.Render` undefined.

- [ ] **Step 4: Implement the renderer**

```go
// internal/markdown/render.go
package markdown

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

var sanitizer = bluemonday.UGCPolicy()

// Render converts Markdown source to sanitized HTML safe to send to
// the client as-is.
func Render(source string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(source), &buf); err != nil {
		return "", err
	}
	return sanitizer.Sanitize(buf.String()), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/markdown/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/markdown go.mod go.sum
git commit -m "feat: add sanitized markdown rendering"
```

---

### Task 5: Note model and repository (CRUD + soft delete + list/filter/pagination)

**Files:**
- Create: `internal/models/note.go`
- Create: `internal/repo/note_repo.go`
- Test: `internal/repo/note_repo_test.go`

**Interfaces:**
- Consumes: `repo.NewUserRepo`, `newTestPool` (Task 3).
- Produces: `models.Note{ID, Title, Content, AuthorID, CreatedAt, UpdatedAt, DeletedAt *time.Time}`; `repo.NoteRepo` with `Create(ctx, title, content string, authorID int64) (*models.Note, error)`, `GetByID(ctx, id int64) (*models.Note, error)`, `Update(ctx, id int64, title, content string) (*models.Note, error)`, `SoftDelete(ctx, id int64) error`, `List(ctx, params repo.NoteListParams) ([]models.Note, int, error)`; `repo.NoteListParams{Search string; AuthorID *int64; Offset, Limit int}`.

- [ ] **Step 1: Write the failing repository test**

```go
// internal/repo/note_repo_test.go
package repo_test

import (
	"context"
	"testing"

	"notebook/internal/models"
	"notebook/internal/repo"
)

func seedUser(t *testing.T, users *repo.UserRepo, subject string) *models.User {
	t.Helper()
	u, err := users.UpsertByOIDCSubject(context.Background(), subject, subject+"@example.com", subject, models.RoleUser)
	if err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	return u
}

func TestNoteRepo_CreateGetUpdateSoftDelete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	author := seedUser(t, users, "author-1")

	created, err := notes.Create(ctx, "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected assigned ID")
	}

	fetched, err := notes.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if fetched.Title != "Title" {
		t.Fatalf("unexpected title: %s", fetched.Title)
	}

	updated, err := notes.Update(ctx, created.ID, "New Title", "New Content")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Title != "New Title" || updated.Content != "New Content" {
		t.Fatalf("update did not apply: %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("expected updated_at to advance")
	}

	if err := notes.SoftDelete(ctx, created.ID); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}
	if _, err := notes.GetByID(ctx, created.ID); err == nil {
		t.Fatalf("expected error fetching soft-deleted note")
	}
}

func TestNoteRepo_List_FiltersSearchAuthorAndPaginates(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	alice := seedUser(t, users, "alice")
	bob := seedUser(t, users, "bob")

	mustCreate := func(title, content string, authorID int64) {
		if _, err := notes.Create(ctx, title, content, authorID); err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}
	mustCreate("Grocery list", "milk, eggs", alice.ID)
	mustCreate("Project plan", "roadmap content", alice.ID)
	mustCreate("Meeting notes", "grocery budget discussion", bob.ID)

	results, total, err := notes.List(ctx, repo.NoteListParams{Search: "grocery", Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 matches for 'grocery' across title+content, got %d", total)
	}
	_ = results

	aliceID := alice.ID
	results, total, err = notes.List(ctx, repo.NoteListParams{AuthorID: &aliceID, Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("list by author failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 notes by alice, got %d", total)
	}

	results, total, err = notes.List(ctx, repo.NoteListParams{Offset: 0, Limit: 1})
	if err != nil {
		t.Fatalf("paginated list failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with limit=1, got %d", len(results))
	}
	if total != 3 {
		t.Fatalf("expected total count 3 regardless of limit, got %d", total)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/repo/... -run TestNoteRepo -v`
Expected: FAIL — `repo.NewNoteRepo` undefined.

- [ ] **Step 3: Implement the Note model and repository**

```go
// internal/models/note.go
package models

import "time"

type Note struct {
	ID        int64
	Title     string
	Content   string
	AuthorID  int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
```

```go
// internal/repo/note_repo.go
package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"notebook/internal/models"
)

var ErrNotFound = errors.New("not found")

type NoteRepo struct {
	pool *pgxpool.Pool
}

func NewNoteRepo(pool *pgxpool.Pool) *NoteRepo {
	return &NoteRepo{pool: pool}
}

func (r *NoteRepo) Create(ctx context.Context, title, content string, authorID int64) (*models.Note, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO notes (title, content, author_id)
		VALUES ($1, $2, $3)
		RETURNING id, title, content, author_id, created_at, updated_at, deleted_at
	`, title, content, authorID)
	return scanNote(row)
}

func (r *NoteRepo) GetByID(ctx context.Context, id int64) (*models.Note, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, title, content, author_id, created_at, updated_at, deleted_at
		FROM notes WHERE id = $1 AND deleted_at IS NULL
	`, id)
	note, err := scanNote(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return note, err
}

func (r *NoteRepo) Update(ctx context.Context, id int64, title, content string) (*models.Note, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE notes SET title = $1, content = $2, updated_at = now()
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, title, content, author_id, created_at, updated_at, deleted_at
	`, title, content, id)
	note, err := scanNote(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return note, err
}

func (r *NoteRepo) SoftDelete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notes SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE comments SET deleted_at = now(), updated_at = now()
		WHERE note_id = $1 AND deleted_at IS NULL
	`, id)
	return err
}

type NoteListParams struct {
	Search   string
	AuthorID *int64
	Offset   int
	Limit    int
}

func (r *NoteRepo) List(ctx context.Context, params NoteListParams) ([]models.Note, int, error) {
	limit := params.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, title, content, author_id, created_at, updated_at, deleted_at
		FROM notes
		WHERE deleted_at IS NULL
		  AND ($1 = '' OR title ILIKE '%' || $1 || '%' OR content ILIKE '%' || $1 || '%')
		  AND ($2::bigint IS NULL OR author_id = $2)
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4
	`, params.Search, params.AuthorID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notes []models.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, 0, err
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FROM notes
		WHERE deleted_at IS NULL
		  AND ($1 = '' OR title ILIKE '%' || $1 || '%' OR content ILIKE '%' || $1 || '%')
		  AND ($2::bigint IS NULL OR author_id = $2)
	`, params.Search, params.AuthorID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	return notes, total, nil
}

func scanNote(row rowScanner) (*models.Note, error) {
	var n models.Note
	if err := row.Scan(&n.ID, &n.Title, &n.Content, &n.AuthorID, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt); err != nil {
		return nil, err
	}
	return &n, nil
}
```

Note: `rowScanner` already exists from Task 3's `user_repo.go`; both
`pgx.Row` and `pgx.Rows` satisfy it since both expose `Scan(...) error`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/repo/... -run TestNoteRepo -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/note.go internal/repo/note_repo.go internal/repo/note_repo_test.go
git commit -m "feat: add Note model and repository with list/filter/pagination"
```

---

### Task 6: Comment model and repository (CRUD + soft delete)

**Files:**
- Create: `internal/models/comment.go`
- Create: `internal/repo/comment_repo.go`
- Test: `internal/repo/comment_repo_test.go`

**Interfaces:**
- Consumes: `repo.NewNoteRepo`, `repo.NewUserRepo` (Tasks 3, 5).
- Produces: `models.Comment{ID, NoteID, Content, AuthorID, CreatedAt, UpdatedAt, DeletedAt *time.Time}`; `repo.CommentRepo` with `Create(ctx, noteID int64, content string, authorID int64) (*models.Comment, error)`, `Update(ctx, id int64, content string) (*models.Comment, error)`, `SoftDelete(ctx, id int64) error`, `ListByNoteID(ctx, noteID int64) ([]models.Comment, error)`.

- [ ] **Step 1: Write the failing repository test**

```go
// internal/repo/comment_repo_test.go
package repo_test

import (
	"context"
	"testing"

	"notebook/internal/repo"
)

func TestCommentRepo_CreateUpdateListSoftDelete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author := seedUser(t, users, "note-author")
	commenter := seedUser(t, users, "commenter")
	note, err := notes.Create(ctx, "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("create note failed: %v", err)
	}

	created, err := comments.Create(ctx, note.ID, "First comment", commenter.ID)
	if err != nil {
		t.Fatalf("create comment failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected assigned ID")
	}

	updated, err := comments.Update(ctx, created.ID, "Edited comment")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Content != "Edited comment" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	list, err := comments.ListByNoteID(ctx, note.ID)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(list))
	}

	if err := comments.SoftDelete(ctx, created.ID); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}

	list, err = comments.ListByNoteID(ctx, note.ID)
	if err != nil {
		t.Fatalf("list after delete failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 visible comments after soft delete, got %d", len(list))
	}
}

func TestNoteRepo_SoftDelete_CascadesToComments(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author := seedUser(t, users, "cascade-author")
	note, err := notes.Create(ctx, "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("create note failed: %v", err)
	}
	if _, err := comments.Create(ctx, note.ID, "A comment", author.ID); err != nil {
		t.Fatalf("create comment failed: %v", err)
	}

	if err := notes.SoftDelete(ctx, note.ID); err != nil {
		t.Fatalf("soft delete note failed: %v", err)
	}

	list, err := comments.ListByNoteID(ctx, note.ID)
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected comments hidden after note soft delete, got %d visible", len(list))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/repo/... -run "TestCommentRepo|TestNoteRepo_SoftDelete_CascadesToComments" -v`
Expected: FAIL — `repo.NewCommentRepo` undefined.

- [ ] **Step 3: Implement the Comment model and repository**

```go
// internal/models/comment.go
package models

import "time"

type Comment struct {
	ID        int64
	NoteID    int64
	Content   string
	AuthorID  int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
```

```go
// internal/repo/comment_repo.go
package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"notebook/internal/models"
)

type CommentRepo struct {
	pool *pgxpool.Pool
}

func NewCommentRepo(pool *pgxpool.Pool) *CommentRepo {
	return &CommentRepo{pool: pool}
}

func (r *CommentRepo) Create(ctx context.Context, noteID int64, content string, authorID int64) (*models.Comment, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO comments (note_id, content, author_id)
		VALUES ($1, $2, $3)
		RETURNING id, note_id, content, author_id, created_at, updated_at, deleted_at
	`, noteID, content, authorID)
	return scanComment(row)
}

func (r *CommentRepo) GetByID(ctx context.Context, id int64) (*models.Comment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, note_id, content, author_id, created_at, updated_at, deleted_at
		FROM comments WHERE id = $1 AND deleted_at IS NULL
	`, id)
	c, err := scanComment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (r *CommentRepo) Update(ctx context.Context, id int64, content string) (*models.Comment, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE comments SET content = $1, updated_at = now()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING id, note_id, content, author_id, created_at, updated_at, deleted_at
	`, content, id)
	c, err := scanComment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (r *CommentRepo) SoftDelete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE comments SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *CommentRepo) ListByNoteID(ctx context.Context, noteID int64) ([]models.Comment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, note_id, content, author_id, created_at, updated_at, deleted_at
		FROM comments
		WHERE note_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, *c)
	}
	return comments, rows.Err()
}

func scanComment(row rowScanner) (*models.Comment, error) {
	var c models.Comment
	if err := row.Scan(&c.ID, &c.NoteID, &c.Content, &c.AuthorID, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt); err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/repo/... -v`
Expected: PASS (all repo tests, including Task 5's)

- [ ] **Step 5: Commit**

```bash
git add internal/models/comment.go internal/repo/comment_repo.go internal/repo/comment_repo_test.go
git commit -m "feat: add Comment model and repository, note soft-delete cascade"
```

---

### Task 7: Auth context stub and authorization helpers

**Files:**
- Create: `internal/api/authctx.go`
- Create: `internal/api/authz.go`
- Test: `internal/api/authz_test.go`

**Interfaces:**
- Consumes: `models.User`, `models.RoleAdmin` (Task 3).
- Produces: `api.CurrentUser(ctx context.Context) (*models.User, bool)`; `api.WithUser(ctx context.Context, u *models.User) context.Context`; `api.StubAuthMiddleware(fixedUser *models.User) mux.MiddlewareFunc` (Plan 2 replaces this middleware with the real OIDC/BFF session one — handlers only ever call `CurrentUser`); `api.isOwnerOrAdmin(user *models.User, authorID int64) bool`; `api.isAdmin(user *models.User) bool`.

- [ ] **Step 1: Write the failing authz test**

```go
// internal/api/authz_test.go
package api

import (
	"testing"

	"notebook/internal/models"
)

func TestIsOwnerOrAdmin(t *testing.T) {
	owner := &models.User{ID: 1, Role: models.RoleUser}
	other := &models.User{ID: 2, Role: models.RoleUser}
	admin := &models.User{ID: 3, Role: models.RoleAdmin}

	if !isOwnerOrAdmin(owner, 1) {
		t.Fatalf("expected owner to be authorized")
	}
	if isOwnerOrAdmin(other, 1) {
		t.Fatalf("expected non-owner, non-admin to be denied")
	}
	if !isOwnerOrAdmin(admin, 1) {
		t.Fatalf("expected admin to be authorized regardless of ownership")
	}
}

func TestIsAdmin(t *testing.T) {
	if isAdmin(&models.User{Role: models.RoleUser}) {
		t.Fatalf("expected regular user to not be admin")
	}
	if !isAdmin(&models.User{Role: models.RoleAdmin}) {
		t.Fatalf("expected admin role to be admin")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run "TestIsOwnerOrAdmin|TestIsAdmin" -v`
Expected: FAIL — `isOwnerOrAdmin`/`isAdmin` undefined.

- [ ] **Step 3: Implement auth context and authorization helpers**

```go
// internal/api/authctx.go
package api

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/models"
)

type ctxKey int

const userCtxKey ctxKey = iota

func WithUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, userCtxKey, u)
}

func CurrentUser(ctx context.Context) (*models.User, bool) {
	u, ok := ctx.Value(userCtxKey).(*models.User)
	return u, ok
}

// StubAuthMiddleware injects a fixed user into every request's context.
// Plan 2 (OIDC/BFF) replaces this with real session-cookie based auth;
// handlers are unaffected because they only ever call CurrentUser.
func StubAuthMiddleware(fixedUser *models.User) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithUser(r.Context(), fixedUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

```go
// internal/api/authz.go
package api

import "notebook/internal/models"

func isOwnerOrAdmin(user *models.User, authorID int64) bool {
	if user == nil {
		return false
	}
	return user.ID == authorID || user.Role == models.RoleAdmin
}

func isAdmin(user *models.User) bool {
	return user != nil && user.Role == models.RoleAdmin
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/... -run "TestIsOwnerOrAdmin|TestIsAdmin" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/authctx.go internal/api/authz.go internal/api/authz_test.go
git commit -m "feat: add auth context accessor and ownership/admin authorization helpers"
```

---

### Task 8: Notes HTTP API

**Files:**
- Create: `internal/api/notes_handler.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/notes_handler_test.go`

**Interfaces:**
- Consumes: `repo.NoteRepo` (Task 5), `CurrentUser`/`isOwnerOrAdmin` (Task 7).
- Produces: `api.NewNotesHandler(notes *repo.NoteRepo) *NotesHandler` with methods `List`, `Create`, `Get`, `Update`, `Delete` registered on routes `GET/POST /api/notes`, `GET/PUT/DELETE /api/notes/{id}`.

- [ ] **Step 1: Write the failing handler tests**

```go
// internal/api/notes_handler_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"notebook/internal/db"
	"notebook/internal/db/migrations"
	"notebook/internal/models"
	"notebook/internal/repo"
)

// newTestServer mirrors internal/repo's testcontainers setup so handler
// tests exercise the real repository stack, not mocks.
func newTestServer(t *testing.T, asUser *models.User) (*mux.Router, *repo.NoteRepo, *models.User) {
	t.Helper()
	pool := newAPITestPool(t)
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)

	author, err := users.UpsertByOIDCSubject(context.Background(), "handler-author", "a@example.com", "Author", models.RoleUser)
	if err != nil {
		t.Fatalf("seed author failed: %v", err)
	}
	user := asUser
	if user == nil {
		user = author
	}

	router := mux.NewRouter()
	router.Use(StubAuthMiddleware(user))
	handler := NewNotesHandler(notes)
	router.HandleFunc("/api/notes", handler.List).Methods(http.MethodGet)
	router.HandleFunc("/api/notes", handler.Create).Methods(http.MethodPost)
	router.HandleFunc("/api/notes/{id}", handler.Get).Methods(http.MethodGet)
	router.HandleFunc("/api/notes/{id}", handler.Update).Methods(http.MethodPut)
	router.HandleFunc("/api/notes/{id}", handler.Delete).Methods(http.MethodDelete)

	return router, notes, author
}

func TestNotesHandler_CreateRejectsBlankTitle(t *testing.T) {
	router, _, _ := newTestServer(t, nil)

	body, _ := json.Marshal(map[string]string{"title": "   ", "content": "content"})
	req := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank title, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotesHandler_CreateThenGet(t *testing.T) {
	router, _, _ := newTestServer(t, nil)

	body, _ := json.Marshal(map[string]string{"title": "My Note", "content": "Hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/notes/"+itoa(created.ID), nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
}

func TestNotesHandler_UpdateByNonAuthorIsForbidden(t *testing.T) {
	router, notes, author := newTestServer(t, nil)
	note, err := notes.Create(context.Background(), "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("seed note failed: %v", err)
	}

	intruder := &models.User{ID: author.ID + 999, Role: models.RoleUser}
	router.Use(StubAuthMiddleware(intruder))

	body, _ := json.Marshal(map[string]string{"title": "Hacked", "content": "Hacked"})
	req := httptest.NewRequest(http.MethodPut, "/api/notes/"+itoa(note.ID), bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
```

Add `"strconv"` to this file's imports (`mux` stays imported, since
`newTestServer` still uses `mux.Router` and `mux.HandleFunc`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestNotesHandler -v`
Expected: FAIL — `NewNotesHandler`, `newAPITestPool` undefined.

- [ ] **Step 3: Add the API-layer testcontainers pool helper**

```go
// internal/api/testutil_test.go
package api

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"notebook/internal/db"
	"notebook/internal/db/migrations"
)

func newAPITestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("notebook"),
		postgres.WithUsername("notebook"),
		postgres.WithPassword("notebook"),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}
	return pool
}
```

- [ ] **Step 4: Implement the notes handler**

```go
// internal/api/notes_handler.go
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"notebook/internal/repo"
)

type NotesHandler struct {
	notes *repo.NoteRepo
}

func NewNotesHandler(notes *repo.NoteRepo) *NotesHandler {
	return &NotesHandler{notes: notes}
}

type noteRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type noteResponse struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	AuthorID  int64  `json:"author_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *NotesHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "title and content are required")
		return
	}

	note, err := h.notes.Create(r.Context(), req.Title, req.Content, user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create note")
		return
	}

	writeJSON(w, http.StatusCreated, noteResponse{
		ID: note.ID, Title: note.Title, Content: note.Content, AuthorID: note.AuthorID,
		CreatedAt: note.CreatedAt.Format(timeLayout), UpdatedAt: note.UpdatedAt.Format(timeLayout),
	})
}

func (h *NotesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	note, err := h.notes.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "note not found")
		return
	}

	writeJSON(w, http.StatusOK, noteResponse{
		ID: note.ID, Title: note.Title, Content: note.Content, AuthorID: note.AuthorID,
		CreatedAt: note.CreatedAt.Format(timeLayout), UpdatedAt: note.UpdatedAt.Format(timeLayout),
	})
}

func (h *NotesHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	existing, err := h.notes.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "note not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may edit this note")
		return
	}

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "title and content are required")
		return
	}

	updated, err := h.notes.Update(r.Context(), id, req.Title, req.Content)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update note")
		return
	}

	writeJSON(w, http.StatusOK, noteResponse{
		ID: updated.ID, Title: updated.Title, Content: updated.Content, AuthorID: updated.AuthorID,
		CreatedAt: updated.CreatedAt.Format(timeLayout), UpdatedAt: updated.UpdatedAt.Format(timeLayout),
	})
}

func (h *NotesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	existing, err := h.notes.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "note not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may delete this note")
		return
	}

	if err := h.notes.SoftDelete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete note")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *NotesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := repo.NoteListParams{
		Search: strings.TrimSpace(q.Get("search")),
		Limit:  50,
	}
	if authorStr := q.Get("author_id"); authorStr != "" {
		authorID, err := strconv.ParseInt(authorStr, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid author_id")
			return
		}
		params.AuthorID = &authorID
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid offset")
			return
		}
		params.Offset = offset
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		params.Limit = limit
	}

	notes, total, err := h.notes.List(r.Context(), params)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list notes")
		return
	}

	items := make([]noteResponse, 0, len(notes))
	for _, n := range notes {
		items = append(items, noteResponse{
			ID: n.ID, Title: n.Title, Content: n.Content, AuthorID: n.AuthorID,
			CreatedAt: n.CreatedAt.Format(timeLayout), UpdatedAt: n.UpdatedAt.Format(timeLayout),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
```

- [ ] **Step 5: Wire the notes routes into the router**

```go
// internal/api/router.go
package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/repo"
)

func NewRouter(logger *slog.Logger, notes *repo.NoteRepo) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)

	notesHandler := NewNotesHandler(notes)
	r.HandleFunc("/api/notes", notesHandler.List).Methods(http.MethodGet)
	r.HandleFunc("/api/notes", notesHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/api/notes/{id}", notesHandler.Get).Methods(http.MethodGet)
	r.HandleFunc("/api/notes/{id}", notesHandler.Update).Methods(http.MethodPut)
	r.HandleFunc("/api/notes/{id}", notesHandler.Delete).Methods(http.MethodDelete)

	return r
}
```

Update `cmd/server/main.go`'s call to `api.NewRouter` to pass a
`*repo.NoteRepo` built from a connected pool (wire `db.Connect` +
`migrations.Up` + `repo.NewNoteRepo` there, using a `DATABASE_URL`
environment variable for the DSN).

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/api cmd/server/main.go
git commit -m "feat: add notes HTTP API with ownership-based authorization"
```

---

### Task 9: Comments HTTP API and Markdown preview endpoint

**Files:**
- Create: `internal/api/comments_handler.go`
- Create: `internal/api/render_handler.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/comments_handler_test.go`
- Test: `internal/api/render_handler_test.go`

**Interfaces:**
- Consumes: `repo.CommentRepo` (Task 6), `markdown.Render` (Task 4), `isOwnerOrAdmin` (Task 7).
- Produces: `api.NewCommentsHandler(comments *repo.CommentRepo) *CommentsHandler` with `List`, `Create`, `Update`, `Delete` on routes `GET/POST /api/notes/{noteId}/comments`, `PUT/DELETE /api/comments/{id}`; `api.RenderPreviewHandler(w, r)` on `POST /api/render-preview`.

- [ ] **Step 1: Write the failing comments handler tests**

```go
// internal/api/comments_handler_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"notebook/internal/models"
	"notebook/internal/repo"
)

func newCommentsTestServer(t *testing.T, asUser *models.User) (*mux.Router, *repo.NoteRepo, *repo.CommentRepo, *models.User) {
	t.Helper()
	pool := newAPITestPool(t)
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author, err := users.UpsertByOIDCSubject(context.Background(), "note-author", "a@example.com", "Author", models.RoleUser)
	if err != nil {
		t.Fatalf("seed author failed: %v", err)
	}
	user := asUser
	if user == nil {
		user = author
	}

	router := mux.NewRouter()
	router.Use(StubAuthMiddleware(user))
	handler := NewCommentsHandler(comments)
	router.HandleFunc("/api/notes/{noteId}/comments", handler.List).Methods(http.MethodGet)
	router.HandleFunc("/api/notes/{noteId}/comments", handler.Create).Methods(http.MethodPost)
	router.HandleFunc("/api/comments/{id}", handler.Update).Methods(http.MethodPut)
	router.HandleFunc("/api/comments/{id}", handler.Delete).Methods(http.MethodDelete)

	return router, notes, comments, author
}

func TestCommentsHandler_CreateThenList(t *testing.T) {
	router, notes, _, author := newCommentsTestServer(t, nil)
	note, err := notes.Create(context.Background(), "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("seed note failed: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"content": "Nice note!"})
	req := httptest.NewRequest(http.MethodPost, "/api/notes/"+itoa(note.ID)+"/comments", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/notes/"+itoa(note.ID)+"/comments", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
}

func TestCommentsHandler_DeleteByNonAuthorIsForbidden(t *testing.T) {
	router, notes, comments, author := newCommentsTestServer(t, nil)
	note, err := notes.Create(context.Background(), "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("seed note failed: %v", err)
	}
	comment, err := comments.Create(context.Background(), note.ID, "A comment", author.ID)
	if err != nil {
		t.Fatalf("seed comment failed: %v", err)
	}

	intruder := &models.User{ID: author.ID + 999, Role: models.RoleUser}
	router.Use(StubAuthMiddleware(intruder))

	req := httptest.NewRequest(http.MethodDelete, "/api/comments/"+itoa(comment.ID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Write the failing render preview test**

```go
// internal/api/render_handler_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderPreviewHandler_RendersSanitizedHTML(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"content": "# Hi <script>alert(1)</script>"})
	req := httptest.NewRequest(http.MethodPost, "/api/render-preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	RenderPreviewHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !strings.Contains(resp.HTML, "<h1>Hi</h1>") {
		t.Fatalf("expected rendered heading, got: %s", resp.HTML)
	}
	if strings.Contains(resp.HTML, "<script>") {
		t.Fatalf("expected script tag stripped, got: %s", resp.HTML)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestCommentsHandler|TestRenderPreviewHandler" -v`
Expected: FAIL — `NewCommentsHandler`, `RenderPreviewHandler` undefined.

- [ ] **Step 4: Implement the comments handler**

```go
// internal/api/comments_handler.go
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"notebook/internal/repo"
)

type CommentsHandler struct {
	comments *repo.CommentRepo
}

func NewCommentsHandler(comments *repo.CommentRepo) *CommentsHandler {
	return &CommentsHandler{comments: comments}
}

type commentRequest struct {
	Content string `json:"content"`
}

type commentResponse struct {
	ID        int64  `json:"id"`
	NoteID    int64  `json:"note_id"`
	Content   string `json:"content"`
	AuthorID  int64  `json:"author_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}


func (h *CommentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	noteID, err := strconv.ParseInt(mux.Vars(r)["noteId"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	var req commentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	c, err := h.comments.Create(r.Context(), noteID, req.Content, user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create comment")
		return
	}

	writeJSON(w, http.StatusCreated, commentResponse{
		ID: c.ID, NoteID: c.NoteID, Content: c.Content, AuthorID: c.AuthorID,
		CreatedAt: c.CreatedAt.Format(timeLayout), UpdatedAt: c.UpdatedAt.Format(timeLayout),
	})
}

func (h *CommentsHandler) List(w http.ResponseWriter, r *http.Request) {
	noteID, err := strconv.ParseInt(mux.Vars(r)["noteId"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid note id")
		return
	}

	list, err := h.comments.ListByNoteID(r.Context(), noteID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	items := make([]commentResponse, 0, len(list))
	for _, c := range list {
		items = append(items, commentResponse{
			ID: c.ID, NoteID: c.NoteID, Content: c.Content, AuthorID: c.AuthorID,
			CreatedAt: c.CreatedAt.Format(timeLayout), UpdatedAt: c.UpdatedAt.Format(timeLayout),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *CommentsHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid comment id")
		return
	}

	existing, err := h.comments.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "comment not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may edit this comment")
		return
	}

	var req commentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	updated, err := h.comments.Update(r.Context(), id, req.Content)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update comment")
		return
	}

	writeJSON(w, http.StatusOK, commentResponse{
		ID: updated.ID, NoteID: updated.NoteID, Content: updated.Content, AuthorID: updated.AuthorID,
		CreatedAt: updated.CreatedAt.Format(timeLayout), UpdatedAt: updated.UpdatedAt.Format(timeLayout),
	})
}

func (h *CommentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid comment id")
		return
	}

	existing, err := h.comments.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "comment not found")
		return
	}
	if !isOwnerOrAdmin(user, existing.AuthorID) {
		writeJSONError(w, http.StatusForbidden, "only the author or an admin may delete this comment")
		return
	}

	if err := h.comments.SoftDelete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete comment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```


- [ ] **Step 5: Implement the render preview handler**

```go
// internal/api/render_handler.go
package api

import (
	"encoding/json"
	"net/http"

	"notebook/internal/markdown"
)

type renderRequest struct {
	Content string `json:"content"`
}

func RenderPreviewHandler(w http.ResponseWriter, r *http.Request) {
	var req renderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	html, err := markdown.Render(req.Content)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to render markdown")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"html": html})
}
```

- [ ] **Step 6: Wire comments and render-preview routes into the router**

```go
// internal/api/router.go
package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/repo"
)

func NewRouter(logger *slog.Logger, notes *repo.NoteRepo, comments *repo.CommentRepo) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/render-preview", RenderPreviewHandler).Methods(http.MethodPost)

	notesHandler := NewNotesHandler(notes)
	r.HandleFunc("/api/notes", notesHandler.List).Methods(http.MethodGet)
	r.HandleFunc("/api/notes", notesHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/api/notes/{id}", notesHandler.Get).Methods(http.MethodGet)
	r.HandleFunc("/api/notes/{id}", notesHandler.Update).Methods(http.MethodPut)
	r.HandleFunc("/api/notes/{id}", notesHandler.Delete).Methods(http.MethodDelete)

	commentsHandler := NewCommentsHandler(comments)
	r.HandleFunc("/api/notes/{noteId}/comments", commentsHandler.List).Methods(http.MethodGet)
	r.HandleFunc("/api/notes/{noteId}/comments", commentsHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/api/comments/{id}", commentsHandler.Update).Methods(http.MethodPut)
	r.HandleFunc("/api/comments/{id}", commentsHandler.Delete).Methods(http.MethodDelete)

	return r
}
```

Update `cmd/server/main.go`'s call site to also build and pass a
`*repo.CommentRepo`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/api cmd/server/main.go
git commit -m "feat: add comments HTTP API and markdown preview endpoint"
```

---

### Task 10: Prometheus metrics middleware and /metrics endpoint

**Files:**
- Create: `internal/metrics/metrics.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/notes_handler.go` (increment `notes_created_total`)
- Modify: `internal/api/comments_handler.go` (increment `comments_created_total`, `comments_deleted_total`)
- Test: `internal/metrics/metrics_test.go`

**Interfaces:**
- Consumes: `api.NewRouter` call site (Task 9).
- Produces: `metrics.Middleware(next http.Handler) http.Handler`; `metrics.Handler() http.Handler` (serves `/metrics`); `metrics.NotesCreatedTotal`, `metrics.CommentsCreatedTotal`, `metrics.CommentsDeletedTotal` (`prometheus.Counter`).

- [ ] **Step 1: Add dependency**

```bash
go get github.com/prometheus/client_golang@v1.20.5
```

- [ ] **Step 2: Write the failing metrics test**

```go
// internal/metrics/metrics_test.go
package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"notebook/internal/metrics"
)

func TestMiddleware_RecordsRequestCount(t *testing.T) {
	handler := metrics.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if !strings.Contains(metricsRec.Body.String(), "http_requests_total") {
		t.Fatalf("expected http_requests_total metric to be present, got body:\n%s", metricsRec.Body.String())
	}
}

func TestBusinessCounters_AreRegistered(t *testing.T) {
	metrics.NotesCreatedTotal.Inc()
	metrics.CommentsCreatedTotal.Inc()
	metrics.CommentsDeletedTotal.Inc()

	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	for _, name := range []string{"notes_created_total", "comments_created_total", "comments_deleted_total"} {
		if !strings.Contains(rec.Body.String(), name) {
			t.Fatalf("expected %s to be present in metrics output", name)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/metrics/... -v`
Expected: FAIL — package `metrics` undefined.

- [ ] **Step 4: Implement metrics collectors and middleware**

```go
// internal/metrics/metrics.go
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests, labeled by route and status code.",
	}, []string{"route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request duration in seconds, labeled by route.",
	}, []string{"route"})

	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Number of HTTP requests currently being served.",
	})

	NotesCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "notes_created_total",
		Help: "Total number of notes created.",
	})

	CommentsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "comments_created_total",
		Help: "Total number of comments created.",
	})

	CommentsDeletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "comments_deleted_total",
		Help: "Total number of comments deleted.",
	})
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		route := r.URL.Path
		httpRequestsTotal.WithLabelValues(route, strconv.Itoa(rec.status)).Inc()
		httpRequestDuration.WithLabelValues(route).Observe(duration)
	})
}

func Handler() http.Handler {
	return promhttp.Handler()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/metrics/... -v`
Expected: PASS

- [ ] **Step 6: Wire the middleware, /metrics route, and business counters**

```go
// internal/api/router.go — add inside NewRouter, wrapping the whole router:
// (full file shown for clarity)
package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"notebook/internal/metrics"
	"notebook/internal/repo"
)

func NewRouter(logger *slog.Logger, notes *repo.NoteRepo, comments *repo.CommentRepo) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/healthz", HealthHandler).Methods(http.MethodGet)
	r.Handle("/metrics", metrics.Handler()).Methods(http.MethodGet)
	r.HandleFunc("/api/render-preview", RenderPreviewHandler).Methods(http.MethodPost)

	notesHandler := NewNotesHandler(notes)
	r.HandleFunc("/api/notes", notesHandler.List).Methods(http.MethodGet)
	r.HandleFunc("/api/notes", notesHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/api/notes/{id}", notesHandler.Get).Methods(http.MethodGet)
	r.HandleFunc("/api/notes/{id}", notesHandler.Update).Methods(http.MethodPut)
	r.HandleFunc("/api/notes/{id}", notesHandler.Delete).Methods(http.MethodDelete)

	commentsHandler := NewCommentsHandler(comments)
	r.HandleFunc("/api/notes/{noteId}/comments", commentsHandler.List).Methods(http.MethodGet)
	r.HandleFunc("/api/notes/{noteId}/comments", commentsHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/api/comments/{id}", commentsHandler.Update).Methods(http.MethodPut)
	r.HandleFunc("/api/comments/{id}", commentsHandler.Delete).Methods(http.MethodDelete)

	return metrics.Middleware(r)
}
```

Note `NewRouter` now returns `http.Handler` instead of `*mux.Router` —
update `cmd/server/main.go`'s `http.ListenAndServe(":8080", router)` call
site; it already accepts an `http.Handler`, so no further change needed
there.

In `internal/api/notes_handler.go`'s `Create` method, add
`metrics.NotesCreatedTotal.Inc()` immediately after the successful
`h.notes.Create(...)` call, before `writeJSON`. In
`internal/api/comments_handler.go`'s `Create` method, add
`metrics.CommentsCreatedTotal.Inc()` the same way; in `Delete`, add
`metrics.CommentsDeletedTotal.Inc()` after the successful
`h.comments.SoftDelete(...)` call. Add `"notebook/internal/metrics"` to
each file's imports.

- [ ] **Step 7: Run the full test suite to verify everything still passes**

Run: `go test ./... -v`
Expected: PASS (all packages)

- [ ] **Step 8: Commit**

```bash
git add internal/metrics internal/api
git commit -m "feat: add Prometheus metrics middleware, /metrics endpoint, and business counters"
```

---

## Out of scope for this plan (covered by later plans)

- Real OIDC/BFF session handling (replaces `StubAuthMiddleware`).
- Vue frontend.
- Helm chart / Kubernetes manifests / Dockerfile.
- kind+Ginkgo and Playwright e2e suites, CI wiring.

## Handoff to Plan 2

Plan 2 (`docs/superpowers/plans/2026-10-19-notebook-oidc-auth.md`) assumes
specific names and signatures from this plan: `api.CurrentUser`,
`api.WithUser`, `api.StubAuthMiddleware`, `api.NewRouter`'s parameter
list, and the `models.User`/`models.Role` types. Once this plan's tasks
are complete and its final whole-branch review has passed, **the
reviewer who signed off on that review must read Plan 2 against what was
actually built** — not what this plan assumed would be built — and
correct any drift (renamed functions, changed signatures, moved files,
changed route paths) in Plan 2's text before Plan 2 execution begins.
This is a required step, not an optional courtesy: Plan 2's tasks are
written as if this plan's interfaces are exactly as specified above, and
small naming drift (e.g. a linter-driven rename) is enough to make Plan
2's code blocks fail to compile against the real codebase.
