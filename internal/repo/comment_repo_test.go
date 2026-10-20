package repo_test

import (
	"context"
	"errors"
	"testing"

	"notebook/internal/repo"
)

func TestCommentRepo_CreateUpdateListSoftDelete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author := seedUser(t, users, "note-author")
	commenter := seedUser(t, users, "commenter")
	note, err := notes.Create(ctx, "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("create note failed: %v", err)
	}

	created, err := comments.Create(ctx, note.ID, "First comment", commenter.ID)
	if err != nil {
		t.Fatalf("create comment failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected assigned ID")
	}

	updated, err := comments.Update(ctx, created.ID, "Edited comment")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Content != "Edited comment" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	list, err := comments.ListByNoteID(ctx, note.ID)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(list))
	}

	if err := comments.SoftDelete(ctx, created.ID); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}

	list, err = comments.ListByNoteID(ctx, note.ID)
	if err != nil {
		t.Fatalf("list after delete failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 visible comments after soft delete, got %d", len(list))
	}
}

func TestNoteRepo_SoftDelete_CascadesToComments(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author := seedUser(t, users, "cascade-author")
	note, err := notes.Create(ctx, "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("create note failed: %v", err)
	}
	if _, err := comments.Create(ctx, note.ID, "A comment", author.ID); err != nil {
		t.Fatalf("create comment failed: %v", err)
	}

	if err := notes.SoftDelete(ctx, note.ID); err != nil {
		t.Fatalf("soft delete note failed: %v", err)
	}

	list, err := comments.ListByNoteID(ctx, note.ID)
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected comments hidden after note soft delete, got %d visible", len(list))
	}
}

func TestCommentRepo_Create_OnSoftDeletedNote_ReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	notes := repo.NewNoteRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author := seedUser(t, users, "deleted-note-author")
	note, err := notes.Create(ctx, "Title", "Content", author.ID)
	if err != nil {
		t.Fatalf("create note failed: %v", err)
	}
	if err := notes.SoftDelete(ctx, note.ID); err != nil {
		t.Fatalf("soft delete note failed: %v", err)
	}

	if _, err := comments.Create(ctx, note.ID, "Too late", author.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound creating a comment on a deleted note, got %v", err)
	}
}

func TestCommentRepo_Create_OnNonexistentNote_ReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := repo.NewUserRepo(pool)
	comments := repo.NewCommentRepo(pool)

	author := seedUser(t, users, "orphan-commenter")

	if _, err := comments.Create(ctx, 999999, "Orphan comment", author.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound creating a comment on a nonexistent note, got %v", err)
	}
}
