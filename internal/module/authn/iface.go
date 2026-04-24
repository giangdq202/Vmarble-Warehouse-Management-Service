package authn

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// LoginInput holds the credentials supplied by the caller.
type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResult is returned on successful authentication.
type LoginResult struct {
	Token     string    `json:"token"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// UserInfo is a lightweight view of a user for cross-module consumption.
type UserInfo struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
}

// UserDetail is a comprehensive view of a user for administrative purposes.
type UserDetail struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// CreateUserInput defines the schema for creating a new user.
type CreateUserInput struct {
	Username string `json:"username" binding:"required,min=3"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role" binding:"required"`
	FullName string `json:"full_name"`
	Email    string `json:"email" binding:"omitempty,email"`
}

// UpdateUserInput defines the schema for updating existing user information.
type UpdateUserInput struct {
	Role     string `json:"role" binding:"required"`
	FullName string `json:"full_name"`
	Email    string `json:"email" binding:"omitempty,email"`
}

// UpdatePasswordInput defines the schema for an administrator to reset a user's password.
type UpdatePasswordInput struct {
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ListUsersParams holds filtering and pagination for user listing.
type ListUsersParams struct {
	httpkit.PageParams
	Roles    []string `json:"roles"`
	IsActive *bool    `json:"is_active"`
}

// Service is the public contract for the authn module.
type Service interface {
	Login(ctx context.Context, in LoginInput) (LoginResult, error)
	GetUser(ctx context.Context, userID uuid.UUID) (UserInfo, error)

	// Admin CRUD operations
	ListUsers(ctx context.Context, params ListUsersParams) (httpkit.PagedResult[UserDetail], error)
	CreateUser(ctx context.Context, creatorID uuid.UUID, in CreateUserInput) (UserDetail, error)
	GetUserDetail(ctx context.Context, userID uuid.UUID) (UserDetail, error)
	UpdateUser(ctx context.Context, userID uuid.UUID, in UpdateUserInput) (UserDetail, error)
	UpdatePassword(ctx context.Context, userID uuid.UUID, in UpdatePasswordInput) error
	DeactivateUser(ctx context.Context, targetID uuid.UUID, actorID uuid.UUID) error
}
