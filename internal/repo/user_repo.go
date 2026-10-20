package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"notebook/internal/models"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

func (r *UserRepo) UpsertByOIDCSubject(ctx context.Context, subject, email, displayName string, role models.Role) (*models.User, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (oidc_subject, email, display_name, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (oidc_subject) DO UPDATE
		SET email = EXCLUDED.email,
		    display_name = EXCLUDED.display_name,
		    role = EXCLUDED.role,
		    updated_at = now()
		RETURNING id, oidc_subject, email, display_name, role, created_at, updated_at
	`, subject, email, displayName, string(role))

	return scanUser(row)
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, oidc_subject, email, display_name, role, created_at, updated_at
		FROM users WHERE id = $1
	`, id)
	return scanUser(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*models.User, error) {
	var u models.User
	var role string
	if err := row.Scan(&u.ID, &u.OIDCSubject, &u.Email, &u.DisplayName, &role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.Role = models.Role(role)
	return &u, nil
}
