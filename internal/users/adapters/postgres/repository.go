// Package postgres implements user persistence using the shared PostgreSQL pool.
package postgres

import (
	"context"
	"errors"

	userdomain "boilerplate/internal/users/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository implements users persistence using a shared PostgreSQL pool.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository injects a pool whose lifetime remains owned by the composition root.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool} }

// Create persists a user and translates known uniqueness violations.
func (r *Repository) Create(ctx context.Context, user userdomain.User) error {
	_, err := r.pool.Exec(
		ctx,
		"INSERT INTO users (id,name,email,created_at) VALUES ($1,$2,$3,$4)",
		user.ID,
		user.Name,
		user.Email,
		user.CreatedAt,
	)

	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" {
		return userdomain.ErrConflict
	}
	return err
}

// FindByID loads a user and translates missing rows into domain errors.
func (r *Repository) FindByID(ctx context.Context, id string) (userdomain.User, error) {
	var user userdomain.User
	err := r.pool.QueryRow(ctx,
		"SELECT id,name,email,created_at FROM users WHERE id=$1", id,
	).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return user, userdomain.ErrNotFound
	}
	return user, err
}

// List returns users ordered by creation time and ID with bounded pagination.
func (r *Repository) List(ctx context.Context, limit, offset int) ([]userdomain.User, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT id,name,email,created_at FROM users ORDER BY created_at,id LIMIT $1 OFFSET $2",
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []userdomain.User{}
	for rows.Next() {
		var user userdomain.User
		if scanErr := rows.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt); scanErr != nil {
			return nil, scanErr
		}
		users = append(users, user)
	}

	return users, rows.Err()
}
