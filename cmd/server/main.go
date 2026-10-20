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

	router := api.NewRouter(logger, notes, comments, api.AuthWiring{
		SessionMiddleware: auth.NewSessionMiddleware(sessionCodec, users),
		LoginHandler:      auth.NewLoginHandler(provider, flowCodec),
		CallbackHandler:   auth.NewCallbackHandler(provider, flowCodec, sessionCodec, users, cfg.OIDC.AdminClaim, cfg.OIDC.AdminClaimValue, "/"),
		LogoutHandler:     auth.NewLogoutHandler(sessionCodec),
	})

	logger.Info("starting server", slog.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", router); err != nil {
		logger.Error("server stopped", slog.String("error", err.Error()))
	}
}
