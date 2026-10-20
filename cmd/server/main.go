package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"notebook/internal/api"
	"notebook/internal/db"
	"notebook/internal/db/migrations"
	"notebook/internal/logging"
	"notebook/internal/repo"
)

func main() {
	logger := logging.New()
	ctx := context.Background()

	pool, err := db.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		return
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		logger.Error("failed to run migrations", slog.String("error", err.Error()))
		return
	}

	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	router := api.NewRouter(logger, notes, comments)

	logger.Info("starting server", slog.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", router); err != nil {
		logger.Error("server stopped", slog.String("error", err.Error()))
	}
}
