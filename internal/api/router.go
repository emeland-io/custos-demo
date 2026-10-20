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
