package authn

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// user is the internal representation of a users row.
type user struct {
	ID           uuid.UUID
	Username     string
	PasswordHash string
	Role         string
	FullName     string
	Email        string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    *time.Time
	CreatedBy    *uuid.UUID
}

// store is the repository interface used only within this module.
type store interface {
	selectUserByUsername(ctx context.Context, username string) (user, error)
	selectUserByID(ctx context.Context, id uuid.UUID) (user, error)
	selectUsers(ctx context.Context, params ListUsersParams) ([]user, int, error)
	insertUser(ctx context.Context, u user) (user, error)
	updateUser(ctx context.Context, u user) (user, error)
	updatePassword(ctx context.Context, userID uuid.UUID, hash string) error
	deactivateUser(ctx context.Context, id uuid.UUID) error
}
