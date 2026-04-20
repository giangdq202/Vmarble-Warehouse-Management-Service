package authn

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

const userSelectFields = `id, username, password_hash, role, COALESCE(full_name, ''), COALESCE(email, ''), is_active, created_at, updated_at, created_by`

func (s *pgStore) scanUser(row pgx.Row) (user, error) {
	var u user
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.FullName, &u.Email, &u.IsActive, &u.CreatedAt, &u.UpdatedAt, &u.CreatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user{}, domain.ErrNotFound
		}
		return user{}, err
	}
	return u, nil
}

func (s *pgStore) selectUserByUsername(ctx context.Context, username string) (user, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+userSelectFields+` FROM users WHERE username = $1`,
		username,
	)
	return s.scanUser(row)
}

func (s *pgStore) selectUserByID(ctx context.Context, id uuid.UUID) (user, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+userSelectFields+` FROM users WHERE id = $1`,
		id,
	)
	return s.scanUser(row)
}

func (s *pgStore) selectAllUsers(ctx context.Context) ([]user, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userSelectFields+` FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []user
	for rows.Next() {
		u, err := s.scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *pgStore) insertUser(ctx context.Context, u user) (user, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, role, full_name, email, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, is_active`,
		u.Username, u.PasswordHash, u.Role, u.FullName, u.Email, u.CreatedBy,
	).Scan(&u.ID, &u.CreatedAt, &u.IsActive)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return user{}, domain.NewBizError(domain.ErrConflict, "username or email already exists")
		}
		return user{}, err
	}
	return u, nil
}

func (s *pgStore) updateUser(ctx context.Context, u user) (user, error) {
	now := time.Now().UTC()
	err := s.pool.QueryRow(ctx,
		`UPDATE users 
		 SET role = $1, full_name = $2, email = $3, updated_at = $4
		 WHERE id = $5
		 RETURNING username, password_hash, is_active, created_at, created_by`,
		u.Role, u.FullName, u.Email, now, u.ID,
	).Scan(&u.Username, &u.PasswordHash, &u.IsActive, &u.CreatedAt, &u.CreatedBy)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user{}, domain.ErrNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return user{}, domain.NewBizError(domain.ErrConflict, "email already exists")
		}
		return user{}, err
	}
	u.UpdatedAt = &now
	return u, nil
}

func (s *pgStore) updatePassword(ctx context.Context, userID uuid.UUID, hash string) error {
	res, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`, hash, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *pgStore) deactivateUser(ctx context.Context, id uuid.UUID) error {
	res, err := s.pool.Exec(ctx, `UPDATE users SET is_active = false, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
