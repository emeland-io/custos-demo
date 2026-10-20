package auth

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"notebook/internal/config"
)

type Provider struct {
	OAuth2Config *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

func NewProvider(ctx context.Context, cfg config.OIDCConfig) (*Provider, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, err
	}

	return &Provider{
		OAuth2Config: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		Verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}
