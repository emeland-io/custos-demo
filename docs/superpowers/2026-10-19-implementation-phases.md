# Notebook App — Implementation Phases

Status overview of the six-plan implementation sequence for the
notebook web app. Each plan lives in `plans/` and argues from the spec
at `specs/2026-10-19-notebook-app.md`. Plans are chained: each one ends
with a "Handoff to Plan N+1" section requiring that plan's reviewer to
re-read the next plan against what was actually built before that
plan's execution starts.

## Status legend

- **Done** — merged to `main`, final whole-branch review passed (fixes applied where needed).
- **Not started** — plan written, implementation not yet begun.

## Phases

| # | Plan | Status | Notes |
|---|------|--------|-------|
| 1 | [Backend domain & API foundations](plans/2026-10-19-notebook-backend-foundations.md) | **Done** | Merged to `main`. DB schema/migrations, User/Note/Comment models+repos, Markdown render+sanitize, notes/comments API with ownership/admin authz, Prometheus metrics, stub auth context. |
| 2 | [OIDC/BFF authentication](plans/2026-10-19-notebook-oidc-auth.md) | **Done** | Merged to `main`. Real OIDC Authorization Code + PKCE flow (BFF pattern), encrypted session + flow cookies, claim-to-role mapping, session middleware replacing Plan 1's stub. |
| 3 | [Vue frontend](plans/2026-10-19-notebook-frontend.md) | **Done** | Note list/detail/editor panes, Pinia stores, `/api/me` addition. |
| 4 | [Container build & static asset embedding](plans/2026-10-19-notebook-container-build.md) | **Done** | `embed.FS` + SPA fallback, multi-stage Dockerfile, local dev compose. |
| 5 | [Helm chart & Kubernetes deployment](plans/2026-10-19-notebook-helm-chart.md) | Not started | Chart, CloudNativePG `Cluster`, migration Job, values/README. |
| 6 | [E2E test framework & CI](plans/2026-10-19-notebook-e2e-ci.md) | Not started | kind+Dex+Ginkgo cluster suite, Playwright frontend suite, GitHub Actions wiring. |

## Phase 1 — completed detail

**Branch:** `notebook/plan-1-backend-foundations` (merged to `main`, deleted).

**Commits merged (12):** 10 task commits plus 2 fix-pass commits from
the final whole-branch review (`3281906` committing `go.mod`/`go.sum`,
`5a774c1` closing the review's Important findings).

**Final review:** fresh reviewer (Opus, general-purpose subagent).
Verdict: "Ready to merge: With fixes." 1 Critical + 2 Important
findings fixed (RED→GREEN each, full suite green before merge); 11
Minor findings deferred; 9 declined-to-judge items reviewed and
accepted as out of scope.

**Fixed before merge:**
- Critical: `go.mod`/`go.sum` were never committed — a clean checkout
  of the task-10 commit failed to build.
- Important: comments were creatable on, and listable for, a
  soft-deleted or nonexistent note (201/500 instead of 404).
- Important: Prometheus `route` label used the raw URL path, giving
  unbounded cardinality — switched to the matched route's path
  template via `mux.CurrentRoute`.

**Deferred minors (11, not acted on, logged in the merge commit and
session history):** nil-user panic risk in the still-stub auth path
(Plan 2 fixes this structurally), pagination `limit` silently clamped
below the stated max instead of to it, response timestamps not forced
to UTC, admin able to edit (not just delete) another user's comment,
swallowed errors / unused `logger` param, `main.go` exits 0 on
startup failure, no request body size limit, unescaped `ILIKE`
wildcards in search, `UserRepo.GetByID` returns raw `pgx.ErrNoRows`
instead of `repo.ErrNotFound`, no guard against a future `go mod tidy`
re-upgrading pinned dependencies, fragile middleware-stacking pattern
in the "intruder" tests.

**Rulings made during implementation** (deviations from the plan's
literal text, each ledgered with cost-if-wrong at the time):
- Pinned `testcontainers-go` and `docker/go-connections` versions that
  `go mod tidy` kept drifting away from.
- Added the `pgx/v5/stdlib` blank import the plan's `migrations.go`
  omitted (`goose.OpenDBWithDriver("pgx", ...)` needs it registered).
- Added an explicit testcontainers wait strategy — the pinned postgres
  module version ships no default one, causing consistent flaky EOF
  failures on first connection — applied to all three testcontainers
  helpers.
- Loosened one render-preview test's exact-string assertion to match
  goldmark+bluemonday's actual (secure) sanitization output.
- Declined to wrap the note→comment soft-delete cascade in a
  transaction, since the `ListByNoteID` join fix already makes a
  mid-cascade failure unobservable through the only code path that
  queries comments by note.

## Phase 2 — completed detail

**Branch:** `worktree-notebook+plan-2-oidc-auth` (merged to `main` via
fast-forward, deleted).

**Commits merged (8):** 7 task commits plus 1 fix-pass commit from the
final whole-branch review (`c86ad48`).

**Handoff verification (required before execution started):** re-read
Plan 2 against the actual Phase 1 codebase. No drift found —
`api.CurrentUser`/`WithUser`/`StubAuthMiddleware`,
`repo.UserRepo.GetByID`/`UpsertByOIDCSubject`, `models.User`/`Role`,
`db.Connect`, `migrations.Up` all matched the plan's assumed
signatures exactly.

**Final review:** fresh reviewer (Opus, general-purpose subagent).
Verdict: "Ready to merge: With fixes." 0 Critical, 6 Important findings
fixed (RED→GREEN each, full suite green before merge); 9 Minor findings
deferred; 9 declined-to-judge items reviewed and accepted as out of
scope.

**Fixed before merge:**
- Important: this plan's router rewrite wrapped `metrics.Middleware`
  from the outside instead of `r.Use(...)`, so `mux.CurrentRoute` never
  saw the matched route and every request was labeled `route="unmatched"`
  — regressing Phase 1's fix for the same metric.
- Important: `SESSION_HASH_KEY`/`SESSION_BLOCK_KEY` were accepted at any
  non-empty length, so a misconfigured key only surfaced as a 500 at
  first login instead of failing config load.
- Important: the PKCE/state flow cookie's 10-minute TTL was enforced
  only by the cookie's own (client-trusted) expiry, not server-side —
  added `securecookie.MaxAge`.
- Important: an id_token with an empty/missing `sub` claim was accepted
  and upserted a user with `oidc_subject=""`, merging distinct
  identities on a misconfigured IdP — switched to `idToken.Subject`
  with a 502 on empty.
- Important: the session middleware reported any database error as 401
  ("authentication required"), masking a transient DB outage as an
  invalid session — now 500 unless the error is "no rows".
- Important: the fake OIDC test server never checked PKCE
  (`code_verifier`/`code_challenge`), so no test actually proved PKCE
  correctness — added S256 verification to the fake's token endpoint.

**Deferred minors (9, not acted on, logged in the merge commit and
session history):** IdP error responses (`?error=...`) reported
misleadingly as "missing authorization code", no OIDC `nonce`, fatal
startup errors exit 0, callback/middleware errors not logged, config
tests depend on a clean shell environment (no explicit clearing of
`OIDC_*`/`SESSION_*`), happy-path test relies on `Set-Cookie` header
order instead of matching by name, the tamper test could flip a
mid-value byte instead of only appending, no router-level wiring test
for `/api/*` vs `/healthz`/`/metrics`/`/auth/*` (partially addressed by
the metrics fix's new test), `ExtractRole` doesn't handle bool or
space-delimited claim shapes.

**Rulings made during implementation** (deviations from the plan's
literal text, each ledgered with cost-if-wrong at the time):
- The plan's own `TestSessionCodec_TamperedCookie_IsRejected` text
  appended "tampered" to the end of the whole `Set-Cookie` line, not the
  cookie value — the garbage landed after unrelated attributes and the
  Cookie-header parser still recovered the untouched value, so the test
  passed for the wrong reason against the plan's own reference
  implementation. Fixed by tampering `cookies[0].Value` directly.
- Declined to fix the `Secure: true` cookie flag vs. Plan 6's e2e client
  hitting `http://127.0.0.1` (whose cookiejar won't treat 127.0.0.1 as a
  secure origin) — `Secure: true` is required by this plan's own Global
  Constraints and not a defect; this is Plan 6's handoff concern to
  resolve (e.g. point its e2e client at `http://localhost` instead).
- Accepted all of the reviewer's declined-to-judge items (no
  server-side session revocation, role-demotion lag up to 24h, no
  RP-initiated IdP logout, logout has no CSRF token, concurrent logins
  in two tabs invalidate the first, `ListenAndServe` has no
  timeouts/graceful shutdown, Plan 3 has no logout UI yet, login is
  GET-triggerable cross-site, state comparison isn't constant-time) as
  consistent with the plan's stated stateless-cookie design or out of
  this plan's scope.

## Phase 3 — completed detail

**Branch:** built directly on `main` (not a dedicated worktree branch).

**Commits (12):** `d8ccc9f` (backend `GET /api/me`) through the frontend
scaffold, API client, Pinia stores (auth/notes/comments), Markdown
editor, note list, note detail + comment thread, the assembled
app shell/router, plus one fix-pass commit (`e877a45`).

**Handoff verification:** re-checked `internal/api/router.go`'s
`NewRouter(logger, notes, comments, auth AuthWiring)` signature against
Plan 4's assumption — matches exactly, no drift.

**Final review:** self-review (no fresh subagent reviewer dispatched),
done at the user's request after the fact rather than as part of guided
execution. Verified directly rather than taken on faith:

- All 15 files the plan specifies exist as named.
- `go test ./internal/api/... -run TestMeHandler` → PASS.
- Frontend: `npm run test` → 39/39 tests pass across all 9 spec files.
- `npm run build` → succeeds, clean production bundle.
- Fix-pass commit `e877a45` addresses the plan's own Review Focus items
  (note-switch state leakage, stale list after mutations, silent
  failures, async races) plus one spec-compliance gap (Markdown
  rendering in comments) — the same shape as Phase 1/2's final-review
  fix passes, but without a documented fresh-reviewer verdict or
  Critical/Important/Minor counts.

**Gap against the Phase 1/2 bar:** no fresh-subagent review verdict is
on record for this phase, and the branch was developed and merged
directly on `main` rather than via an isolated feature branch. Treat
Phase 3 as functionally complete and verified, but procedurally lighter
than Phases 1–2.

## Phase 4 — completed detail

**Branch:** `worktree-notebook-plan-4-container-build`, built in an
isolated `git worktree` at
`/Users/sage/git/custos-demo-worktrees/notebook-plan-4-container-build`
(not yet merged to `main` — see "Rulings" below).

**Commits merged (6):** 5 task commits (`0df0f00` embedded asset
package, `06e7f7f` SPA fallback handler, `9fd70cd` router/main.go
wiring, `01cf3e5` Dockerfile/Makefile, `025e645` docker-compose +
README) plus 1 fix-pass commit (`617d1fa`) from the final whole-branch
review.

**Handoff verification:** re-read Plan 4 against the actual Plan 1–3
codebase before starting — `api.NewRouter`'s signature
(`logger, notes, comments, auth AuthWiring`) matched exactly, no drift.

**Final review:** fresh reviewer (Opus, general-purpose subagent),
dispatched with the plan's Review Focus section verbatim and the
implementer's rulings for it to weigh. Verdict: "Ready to merge with
fixes." 0 Critical, 2 Important findings fixed (RED→GREEN each, full
suite green before merge); 6 Minor findings deferred; all Review Focus
items and all 3 non-README rulings confirmed correct independently
(including a live metrics-label check on the new SPA catch-all route).

**Fixed before merge:**

- Important: any unmatched `/api/*` path or method (e.g. `PATCH
  /api/notes`, `GET /auth/logout`) fell through to the SPA catch-all
  and returned `200 text/html` instead of a 404/405 — the frontend's
  `apiFetch()` would call `response.json()` on HTML and throw a raw
  `SyntaxError` instead of a clean `ApiError`. Fixed by giving the
  `/api` and `/auth` subrouters their own JSON `NotFoundHandler`/
  `MethodNotAllowedHandler`.
- Important: `make frontend-dist`'s `rm -rf internal/webassets/dist`
  deleted the git-tracked `README.txt` placeholder; committing that
  state would remove the directory from git entirely and break `go
  build ./...` on every fresh clone — exactly the failure the
  placeholder exists to prevent. Fixed to clear everything except
  `README.txt` and restore it via `git checkout` as a backstop.

**Deferred minors (6, not acted on, logged in the merge commit and
session history):** `GET /assets/` (and bare `/assets`) exposes an
`http.FileServer` directory listing instead of 404/redirect; a dotted
navigational path like `/notes/v1.2` would 404 by the "has an
extension" heuristic (no current route hits this); the Postgres
`5432` port collision may be silent rather than a visible bind failure
on macOS (Homebrew Postgres on `127.0.0.1` + Docker on `0.0.0.0` can
both bind; symptom becomes an auth failure, not a startup error);
README's local-dev step 3 doesn't give a concrete `DATABASE_URL`
value matching the compose credentials, and references Plan 6's
Dex environment before it exists; `.dockerignore` doesn't exclude
`bin/`/`.env*`/`.DS_Store` (harmless for the shipped
`distroless/static` image, since only the final binary is copied into
it, but worth tightening); no automated guard (e.g. a `RUN test -f
.../index.html` build step) against a future Dockerfile copy-order
regression silently shipping the placeholder instead of the real
frontend.

**Rulings made during implementation** (deviations from the plan's
literal text, each ledgered with cost-if-wrong at the time):

- No tracked root `.gitignore` existed in the committed repo (verified
  via `git ls-files`) — created one fresh with just the
  `internal/webassets/dist` rule instead of "appending" to a file that
  didn't exist in git history. Cost if wrong: trivial merge later.
- Kept the existing `r.Use(metrics.Middleware)` pattern instead of the
  plan's own code snippet (`return metrics.Middleware(r)`), because the
  latter is the exact bug Phase 2's final review fixed (it broke
  `mux.CurrentRoute`, mislabeling every request's route as
  `"unmatched"` in Prometheus). Confirmed by the fresh reviewer: the
  existing `TestNewRouter_MetricsLabelByRouteTemplate_NotUnmatched`
  still passes, and the new SPA catch-all route gets a correctly
  templated (not `"unmatched"`) metrics label.
- Pinned `golang:1.27-alpine` in the Dockerfile instead of the plan's
  `golang:1.23-alpine`, since `go.mod` requires `go 1.27.1`. Confirmed
  by the fresh reviewer as necessary and sufficient — no other Go
  version pin exists anywhere else in the repo to drift from.
- A gorilla/mux (v1.8.1) library quirk: a path-mismatched sibling route
  with a `{variable}` segment (e.g. `/notes/{id}`) can clear a carried
  `ErrMethodMismatch` marker before the router reaches its final
  404-vs-405 decision, so `PATCH /api/notes` returns 404 instead of the
  "more correct" 405. Ruled acceptable as-is: both are JSON, neither is
  the SPA shell, and that distinction (not the exact 4xx code) is what
  the frontend's error handling actually depends on. Cost if wrong:
  would need either a mux upgrade or a hand-rolled method-matching
  shim.
- **Not yet merged to `main`:** this phase was implemented in an
  isolated worktree per explicit instruction, unlike Phase 3 (built
  directly on `main`). The human partner has not yet been asked to
  choose a merge strategy (`superpowers:finishing-a-development-branch`)
  — doing so, and reconciling the worktree's plain README/`.gitignore`
  with `main`'s pre-existing uncommitted versions of both files (richer
  README content, different `.gitignore` rule), remains open.

## Next up

Plan 5 (Helm chart & Kubernetes deployment) is next. Its handoff note
requires confirming this phase's image exposes port `8080`, serves
`/healthz` for liveness/readiness probes, serves `/metrics` for
Prometheus scraping, and reads all configuration exclusively from
environment variables (no config file, no flags) before execution
begins — all true as built. Resolving Phase 4's merge-to-`main`
question (see "Rulings" above) should happen first, since Plan 5 will
assume this phase's Dockerfile/image exist on `main`.
