package authn

import (
	"context"

	"github.com/google/uuid"
)

// user is the internal representation of a users row.
type user struct {
	ID           uuid.UUID
	Username     string
	PasswordHash string
	Role         string
	IsActive     bool
}

// store is the repository interface used only within this module.
type store interface {
	selectUserByUsername(ctx context.Context, username string) (user, error)
	selectUserByID(ctx context.Context, id uuid.UUID) (user, error)
}
