package config_test

import (
	"testing"

	"notebook/internal/config"
)

func TestLoad_AllRequiredVarsSet_Succeeds(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "https://idp.example.com")
	t.Setenv("OIDC_CLIENT_ID", "client-id")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://app.example.com/auth/callback")
	t.Setenv("SESSION_HASH_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_BLOCK_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "postgres://x")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.OIDC.IssuerURL != "https://idp.example.com" {
		t.Fatalf("unexpected issuer url: %s", cfg.OIDC.IssuerURL)
	}
	if cfg.OIDC.AdminClaim != "groups" {
		t.Fatalf("expected default admin claim 'groups', got %s", cfg.OIDC.AdminClaim)
	}
	if cfg.OIDC.AdminClaimValue != "notebook-admin" {
		t.Fatalf("expected default admin claim value, got %s", cfg.OIDC.AdminClaimValue)
	}
}

func TestLoad_MissingOIDCVars_Fails(t *testing.T) {
	t.Setenv("SESSION_HASH_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_BLOCK_KEY", "0123456789abcdef0123456789abcdef")

	if _, err := config.Load(); err == nil {
		t.Fatalf("expected error when OIDC vars are missing")
	}
}

func TestLoad_WrongLengthBlockKey_Fails(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "https://idp.example.com")
	t.Setenv("OIDC_CLIENT_ID", "client-id")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://app.example.com/auth/callback")
	t.Setenv("SESSION_HASH_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_BLOCK_KEY", "too-short")

	if _, err := config.Load(); err == nil {
		t.Fatalf("expected error for a session block key that isn't 16/24/32 bytes")
	}
}

func TestLoad_TooShortHashKey_Fails(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "https://idp.example.com")
	t.Setenv("OIDC_CLIENT_ID", "client-id")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://app.example.com/auth/callback")
	t.Setenv("SESSION_HASH_KEY", "too-short")
	t.Setenv("SESSION_BLOCK_KEY", "0123456789abcdef0123456789abcdef")

	if _, err := config.Load(); err == nil {
		t.Fatalf("expected error for a session hash key shorter than 32 bytes")
	}
}

func TestLoad_MissingSessionKeys_Fails(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "https://idp.example.com")
	t.Setenv("OIDC_CLIENT_ID", "client-id")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://app.example.com/auth/callback")

	if _, err := config.Load(); err == nil {
		t.Fatalf("expected error when session keys are missing")
	}
}
