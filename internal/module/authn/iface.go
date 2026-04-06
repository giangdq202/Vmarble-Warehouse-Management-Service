package authn

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// LoginInput holds the credentials supplied by the caller.
type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResult is returned on successful authentication.
type LoginResult struct {
	Token     string    `json:"token"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// UserInfo is a lightweight view of a user for cross-module consumption.
type UserInfo struct {
	ID   uuid.UUID `json:"id"`
	Role string    `json:"role"`
}

// Service is the public contract for the authn module.
type Service interface {
	Login(ctx context.Context, in LoginInput) (LoginResult, error)
	GetUser(ctx context.Context, userID uuid.UUID) (UserInfo, error)
}
