package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"notebook/internal/models"
)

type CommentRepo struct {
	pool *pgxpool.Pool
}

func NewCommentRepo(pool *pgxpool.Pool) *CommentRepo {
	return &CommentRepo{pool: pool}
}

func (r *CommentRepo) Create(ctx context.Context, noteID int64, content string, authorID int64) (*models.Comment, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO comments (note_id, content, author_id)
		VALUES ($1, $2, $3)
		RETURNING id, note_id, content, author_id, created_at, updated_at, deleted_at
	`, noteID, content, authorID)
	return scanComment(row)
}

func (r *CommentRepo) GetByID(ctx context.Context, id int64) (*models.Comment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, note_id, content, author_id, created_at, updated_at, deleted_at
		FROM comments WHERE id = $1 AND deleted_at IS NULL
	`, id)
	c, err := scanComment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (r *CommentRepo) Update(ctx context.Context, id int64, content string) (*models.Comment, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE comments SET content = $1, updated_at = now()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING id, note_id, content, author_id, created_at, updated_at, deleted_at
	`, content, id)
	c, err := scanComment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (r *CommentRepo) SoftDelete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE comments SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *CommentRepo) ListByNoteID(ctx context.Context, noteID int64) ([]models.Comment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, note_id, content, author_id, created_at, updated_at, deleted_at
		FROM comments
		WHERE note_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, *c)
	}
	return comments, rows.Err()
}

func scanComment(row rowScanner) (*models.Comment, error) {
	var c models.Comment
	if err := row.Scan(&c.ID, &c.NoteID, &c.Content, &c.AuthorID, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt); err != nil {
		return nil, err
	}
	return &c, nil
}
