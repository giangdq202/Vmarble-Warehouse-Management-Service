package authn

import (
	"context"
	"time"
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

// Service is the public contract for the authn module.
type Service interface {
	Login(ctx context.Context, in LoginInput) (LoginResult, error)
}
