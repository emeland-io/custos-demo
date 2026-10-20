package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"notebook/internal/models"
)

var ErrNotFound = errors.New("not found")

type NoteRepo struct {
	pool *pgxpool.Pool
}

func NewNoteRepo(pool *pgxpool.Pool) *NoteRepo {
	return &NoteRepo{pool: pool}
}

func (r *NoteRepo) Create(ctx context.Context, title, content string, authorID int64) (*models.Note, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO notes (title, content, author_id)
		VALUES ($1, $2, $3)
		RETURNING id, title, content, author_id, created_at, updated_at, deleted_at
	`, title, content, authorID)
	return scanNote(row)
}

func (r *NoteRepo) GetByID(ctx context.Context, id int64) (*models.Note, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, title, content, author_id, created_at, updated_at, deleted_at
		FROM notes WHERE id = $1 AND deleted_at IS NULL
	`, id)
	note, err := scanNote(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return note, err
}

func (r *NoteRepo) Update(ctx context.Context, id int64, title, content string) (*models.Note, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE notes SET title = $1, content = $2, updated_at = now()
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, title, content, author_id, created_at, updated_at, deleted_at
	`, title, content, id)
	note, err := scanNote(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return note, err
}

func (r *NoteRepo) SoftDelete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notes SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE comments SET deleted_at = now(), updated_at = now()
		WHERE note_id = $1 AND deleted_at IS NULL
	`, id)
	return err
}

type NoteListParams struct {
	Search   string
	AuthorID *int64
	Offset   int
	Limit    int
}

func (r *NoteRepo) List(ctx context.Context, params NoteListParams) ([]models.Note, int, error) {
	limit := params.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, title, content, author_id, created_at, updated_at, deleted_at
		FROM notes
		WHERE deleted_at IS NULL
		  AND ($1 = '' OR title ILIKE '%' || $1 || '%' OR content ILIKE '%' || $1 || '%')
		  AND ($2::bigint IS NULL OR author_id = $2)
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4
	`, params.Search, params.AuthorID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notes []models.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, 0, err
		}
		notes = append(notes, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FROM notes
		WHERE deleted_at IS NULL
		  AND ($1 = '' OR title ILIKE '%' || $1 || '%' OR content ILIKE '%' || $1 || '%')
		  AND ($2::bigint IS NULL OR author_id = $2)
	`, params.Search, params.AuthorID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	return notes, total, nil
}

func scanNote(row rowScanner) (*models.Note, error) {
	var n models.Note
	if err := row.Scan(&n.ID, &n.Title, &n.Content, &n.AuthorID, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt); err != nil {
		return nil, err
	}
	return &n, nil
}
