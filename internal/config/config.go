package config

import (
	"fmt"
	"os"
)

type OIDCConfig struct {
	IssuerURL       string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	AdminClaim      string
	AdminClaimValue string
}

type SessionConfig struct {
	HashKey  []byte
	BlockKey []byte
}

type Config struct {
	OIDC        OIDCConfig
	Session     SessionConfig
	DatabaseURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		OIDC: OIDCConfig{
			IssuerURL:       os.Getenv("OIDC_ISSUER_URL"),
			ClientID:        os.Getenv("OIDC_CLIENT_ID"),
			ClientSecret:    os.Getenv("OIDC_CLIENT_SECRET"),
			RedirectURL:     os.Getenv("OIDC_REDIRECT_URL"),
			AdminClaim:      envOr("OIDC_ADMIN_CLAIM", "groups"),
			AdminClaimValue: envOr("OIDC_ADMIN_CLAIM_VALUE", "notebook-admin"),
		},
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if cfg.OIDC.IssuerURL == "" || cfg.OIDC.ClientID == "" || cfg.OIDC.ClientSecret == "" || cfg.OIDC.RedirectURL == "" {
		return nil, fmt.Errorf("OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, and OIDC_REDIRECT_URL must all be set")
	}

	hashKey := os.Getenv("SESSION_HASH_KEY")
	blockKey := os.Getenv("SESSION_BLOCK_KEY")
	if hashKey == "" || blockKey == "" {
		return nil, fmt.Errorf("SESSION_HASH_KEY and SESSION_BLOCK_KEY must both be set")
	}
	cfg.Session.HashKey = []byte(hashKey)
	cfg.Session.BlockKey = []byte(blockKey)

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
