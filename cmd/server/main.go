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
