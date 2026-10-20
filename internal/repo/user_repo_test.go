package repo_test

import (
	"context"
	"testing"

	"notebook/internal/models"
	"notebook/internal/repo"
)

func TestUserRepo_UpsertByOIDCSubject_CreatesThenUpdates(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)

	created, err := users.UpsertByOIDCSubject(ctx, "sub-123", "a@example.com", "Alice", models.RoleUser)
	if err != nil {
		t.Fatalf("create upsert failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected assigned ID, got 0")
	}
	if created.Role != models.RoleUser {
		t.Fatalf("expected role %q, got %q", models.RoleUser, created.Role)
	}

	updated, err := users.UpsertByOIDCSubject(ctx, "sub-123", "a@example.com", "Alice Updated", models.RoleAdmin)
	if err != nil {
		t.Fatalf("update upsert failed: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("expected same ID on upsert, got %d vs %d", updated.ID, created.ID)
	}
	if updated.DisplayName != "Alice Updated" || updated.Role != models.RoleAdmin {
		t.Fatalf("expected updated display name/role, got %+v", updated)
	}

	fetched, err := users.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
	if fetched.Email != "a@example.com" {
		t.Fatalf("unexpected email: %s", fetched.Email)
	}
}
