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
