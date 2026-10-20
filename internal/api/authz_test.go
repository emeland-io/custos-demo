package api

import (
	"testing"

	"notebook/internal/models"
)

func TestIsOwnerOrAdmin(t *testing.T) {
	owner := &models.User{ID: 1, Role: models.RoleUser}
	other := &models.User{ID: 2, Role: models.RoleUser}
	admin := &models.User{ID: 3, Role: models.RoleAdmin}

	if !isOwnerOrAdmin(owner, 1) {
		t.Fatalf("expected owner to be authorized")
	}
	if isOwnerOrAdmin(other, 1) {
		t.Fatalf("expected non-owner, non-admin to be denied")
	}
	if !isOwnerOrAdmin(admin, 1) {
		t.Fatalf("expected admin to be authorized regardless of ownership")
	}
}

func TestIsAdmin(t *testing.T) {
	if isAdmin(&models.User{Role: models.RoleUser}) {
		t.Fatalf("expected regular user to not be admin")
	}
	if !isAdmin(&models.User{Role: models.RoleAdmin}) {
		t.Fatalf("expected admin role to be admin")
	}
}
