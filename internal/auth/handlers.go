package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"notebook/internal/repo"
)

const sessionTTL = 24 * time.Hour

func NewLoginHandler(provider *Provider, flowCodec *FlowCodec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := NewState()
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to start login")
			return
		}
		verifier, err := NewPKCEVerifier()
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to start login")
			return
		}

		if err := flowCodec.SetFlow(w, state, verifier); err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to start login")
			return
		}

		authURL := provider.OAuth2Config.AuthCodeURL(state,
			oauth2.SetAuthURLParam("code_challenge", PKCEChallenge(verifier)),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

func NewCallbackHandler(
	provider *Provider,
	flowCodec *FlowCodec,
	sessionCodec *SessionCodec,
	users *repo.UserRepo,
	adminClaim, adminValue, postLoginRedirect string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedState, verifier, ok := flowCodec.ReadFlow(r)
		if !ok {
			writeAuthError(w, http.StatusBadRequest, "login session expired or missing, please try again")
			return
		}
		flowCodec.ClearFlow(w)

		if r.URL.Query().Get("state") != expectedState {
			writeAuthError(w, http.StatusBadRequest, "state mismatch")
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			writeAuthError(w, http.StatusBadRequest, "missing authorization code")
			return
		}

		token, err := provider.OAuth2Config.Exchange(r.Context(), code,
			oauth2.SetAuthURLParam("code_verifier", verifier),
		)
		if err != nil {
			writeAuthError(w, http.StatusBadRequest, "failed to exchange authorization code")
			return
		}

		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok {
			writeAuthError(w, http.StatusBadGateway, "identity provider did not return an id_token")
			return
		}

		idToken, err := provider.Verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			writeAuthError(w, http.StatusBadGateway, "failed to verify id_token")
			return
		}

		var claims map[string]any
		if err := idToken.Claims(&claims); err != nil {
			writeAuthError(w, http.StatusBadGateway, "failed to parse id_token claims")
			return
		}

		subject, _ := claims["sub"].(string)
		email, _ := claims["email"].(string)
		displayName, _ := claims["name"].(string)
		if displayName == "" {
			displayName = email
		}
		role := ExtractRole(claims, adminClaim, adminValue)

		user, err := users.UpsertByOIDCSubject(r.Context(), subject, email, displayName, role)
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to persist user")
			return
		}

		if err := sessionCodec.SetSession(w, user.ID, sessionTTL); err != nil {
			writeAuthError(w, http.StatusInternalServerError, "failed to create session")
			return
		}

		http.Redirect(w, r, postLoginRedirect, http.StatusFound)
	}
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
