# custos-demo
stand up a complex environment to show custos operations.

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
