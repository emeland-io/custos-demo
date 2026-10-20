package auth

import (
	"net/http"

	"notebook/internal/api"
	"notebook/internal/repo"
)

func NewSessionMiddleware(sessionCodec *SessionCodec, users *repo.UserRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := sessionCodec.ReadSession(r)
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			user, err := users.GetByID(r.Context(), userID)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			ctx := api.WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func NewLogoutHandler(sessionCodec *SessionCodec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionCodec.ClearSession(w)
		w.WriteHeader(http.StatusNoContent)
	}
}
