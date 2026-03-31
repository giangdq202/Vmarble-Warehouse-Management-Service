package authn

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type pgStore struct {
	pool *pgxpool.Pool
}

// NewPGStore returns a store backed by PostgreSQL.
func NewPGStore(pool *pgxpool.Pool) store {
	return &pgStore{pool: pool}
}

func (s *pgStore) selectUserByUsername(ctx context.Context, username string) (user, error) {
	var u user
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user{}, domain.ErrNotFound
		}
		return user{}, err
	}
	return u, nil
}
