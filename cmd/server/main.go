package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

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

	if err := run(logger); err != nil {
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", slog.String("error", err.Error()))
		return fmt.Errorf("invalid configuration: %w", err)
	}

	migrateOnly := flag.Bool("migrate-only", false, "run database migrations then exit")
	flag.Parse()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		logger.Error("failed to run migrations", slog.String("error", err.Error()))
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	if *migrateOnly {
		logger.Info("migrations applied, exiting due to --migrate-only")
		return nil
	}

	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	provider, err := auth.NewProvider(ctx, cfg.OIDC)
	if err != nil {
		logger.Error("failed to initialize OIDC provider", slog.String("error", err.Error()))
		return fmt.Errorf("failed to initialize OIDC provider: %w", err)
	}
	flowCodec := auth.NewFlowCodec(cfg.Session.HashKey, cfg.Session.BlockKey)
	sessionCodec := auth.NewSessionCodec(cfg.Session.HashKey, cfg.Session.BlockKey)

	assets, err := webassets.FS()
	if err != nil {
		logger.Error("failed to load embedded frontend assets", slog.String("error", err.Error()))
		return fmt.Errorf("failed to load embedded frontend assets: %w", err)
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
		return fmt.Errorf("server stopped: %w", err)
	}
	return nil
}
