package authn

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
)

const tokenTTL = 24 * time.Hour

type service struct {
	st     store
	secret string
}

// NewService returns an authn Service.
// secret is the same AUTH_SECRET used by auth.Middleware so that tokens
// issued here can be verified by the shared middleware.
func NewService(st store, secret string) Service {
	return &service{st: st, secret: secret}
}

func (s *service) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	if in.Username == "" {
		return LoginResult{}, domain.NewBizError(domain.ErrInvalidInput, "username is required")
	}
	if in.Password == "" {
		return LoginResult{}, domain.NewBizError(domain.ErrInvalidInput, "password is required")
	}

	u, err := s.st.selectUserByUsername(ctx, in.Username)
	if err != nil {
		// Map "not found" to a generic error so callers cannot enumerate usernames.
		return LoginResult{}, domain.NewBizError(domain.ErrInvalidInput, "invalid username or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)); err != nil {
		return LoginResult{}, domain.NewBizError(domain.ErrInvalidInput, "invalid username or password")
	}

	if !u.IsActive {
		return LoginResult{}, domain.NewBizError(domain.ErrPreconditionFailed, "account is disabled")
	}

	exp := time.Now().Add(tokenTTL)
	token := auth.SignToken(s.secret, u.ID.String(), auth.Role(u.Role), exp)

	return LoginResult{
		Token:     "Bearer " + token,
		Role:      u.Role,
		ExpiresAt: exp.UTC(),
	}, nil
}

func (s *service) GetUser(ctx context.Context, userID uuid.UUID) (UserInfo, error) {
	u, err := s.st.selectUserByID(ctx, userID)
	if err != nil {
		return UserInfo{}, err
	}
	return UserInfo{ID: u.ID, Username: u.Username, Role: u.Role}, nil
}
