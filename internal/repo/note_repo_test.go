package repo_test

import (
	"context"
	"testing"

	"notebook/internal/models"
	"notebook/internal/repo"
)

func seedUser(t *testing.T, users *repo.UserRepo, subject string) *models.User {
	t.Helper()
	u, err := users.UpsertByOIDCSubject(context.Background(), subject, subject+"@example.com", subject, models.RoleUser)
	if err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	return u
}

func TestNoteRepo_CreateGetUpdateSoftDelete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	author := seedUser(t, users, "author-1")

	created, err := notes.Create(ctx, "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected assigned ID")
	}

	fetched, err := notes.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if fetched.Title != "Title" {
		t.Fatalf("unexpected title: %s", fetched.Title)
	}

	updated, err := notes.Update(ctx, created.ID, "New Title", "New Content")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Title != "New Title" || updated.Content != "New Content" {
		t.Fatalf("update did not apply: %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("expected updated_at to advance")
	}

	if err := notes.SoftDelete(ctx, created.ID); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}
	if _, err := notes.GetByID(ctx, created.ID); err == nil {
		t.Fatalf("expected error fetching soft-deleted note")
	}
}

func TestNoteRepo_List_FiltersSearchAuthorAndPaginates(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	alice := seedUser(t, users, "alice")
	bob := seedUser(t, users, "bob")

	mustCreate := func(title, content string, authorID int64) {
		if _, err := notes.Create(ctx, title, content, authorID); err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}
	mustCreate("Grocery list", "milk, eggs", alice.ID)
	mustCreate("Project plan", "roadmap content", alice.ID)
	mustCreate("Meeting notes", "grocery budget discussion", bob.ID)

	results, total, err := notes.List(ctx, repo.NoteListParams{Search: "grocery", Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 matches for 'grocery' across title+content, got %d", total)
	}
	_ = results

	aliceID := alice.ID
	results, total, err = notes.List(ctx, repo.NoteListParams{AuthorID: &aliceID, Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("list by author failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 notes by alice, got %d", total)
	}

	results, total, err = notes.List(ctx, repo.NoteListParams{Offset: 0, Limit: 1})
	if err != nil {
		t.Fatalf("paginated list failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with limit=1, got %d", len(results))
	}
	if total != 3 {
		t.Fatalf("expected total count 3 regardless of limit, got %d", total)
	}
}
