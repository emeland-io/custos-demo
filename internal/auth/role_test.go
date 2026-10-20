package auth_test

import (
	"testing"

	"notebook/internal/auth"
	"notebook/internal/models"
)

func TestExtractRole_StringClaimMatches(t *testing.T) {
	claims := map[string]any{"groups": "notebook-admin"}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleAdmin {
		t.Fatalf("expected admin role, got %s", got)
	}
}

func TestExtractRole_ArrayClaimContainsValue(t *testing.T) {
	claims := map[string]any{"groups": []any{"other-group", "notebook-admin"}}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleAdmin {
		t.Fatalf("expected admin role, got %s", got)
	}
}

func TestExtractRole_ArrayClaimWithoutValue_IsUser(t *testing.T) {
	claims := map[string]any{"groups": []any{"other-group"}}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleUser {
		t.Fatalf("expected user role, got %s", got)
	}
}

func TestExtractRole_MissingClaim_IsUser(t *testing.T) {
	claims := map[string]any{}
	if got := auth.ExtractRole(claims, "groups", "notebook-admin"); got != models.RoleUser {
		t.Fatalf("expected user role, got %s", got)
	}
}
