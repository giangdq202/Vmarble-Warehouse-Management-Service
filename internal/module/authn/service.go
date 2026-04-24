package authn

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const tokenTTL = 24 * time.Hour
const bcryptCost = 12

type service struct {
	st     store
	secret string
}

// NewService returns an authn Service.
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

func (s *service) ListUsers(ctx context.Context, params ListUsersParams) (httpkit.PagedResult[UserDetail], error) {
	users, total, err := s.st.selectUsers(ctx, params)
	if err != nil {
		return httpkit.PagedResult[UserDetail]{}, err
	}
	out := make([]UserDetail, len(users))
	for i, u := range users {
		out[i] = s.mapUserToDetail(u)
	}
	return httpkit.NewPagedResult(out, total, params.PageParams), nil
}

func (s *service) CreateUser(ctx context.Context, creatorID uuid.UUID, in CreateUserInput) (UserDetail, error) {
	if err := s.validateRole(in.Role); err != nil {
		return UserDetail{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcryptCost)
	if err != nil {
		return UserDetail{}, err
	}

	u := user{
		Username:     in.Username,
		PasswordHash: string(hash),
		Role:         in.Role,
		FullName:     in.FullName,
		Email:        in.Email,
		CreatedBy:    &creatorID,
	}

	saved, err := s.st.insertUser(ctx, u)
	if err != nil {
		return UserDetail{}, err
	}

	return s.mapUserToDetail(saved), nil
}

func (s *service) GetUserDetail(ctx context.Context, userID uuid.UUID) (UserDetail, error) {
	u, err := s.st.selectUserByID(ctx, userID)
	if err != nil {
		return UserDetail{}, err
	}
	return s.mapUserToDetail(u), nil
}

func (s *service) UpdateUser(ctx context.Context, userID uuid.UUID, in UpdateUserInput) (UserDetail, error) {
	if err := s.validateRole(in.Role); err != nil {
		return UserDetail{}, err
	}

	u := user{
		ID:       userID,
		Role:     in.Role,
		FullName: in.FullName,
		Email:    in.Email,
	}

	updated, err := s.st.updateUser(ctx, u)
	if err != nil {
		return UserDetail{}, err
	}

	return s.mapUserToDetail(updated), nil
}

func (s *service) UpdatePassword(ctx context.Context, userID uuid.UUID, in UpdatePasswordInput) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcryptCost)
	if err != nil {
		return err
	}
	return s.st.updatePassword(ctx, userID, string(hash))
}

func (s *service) DeactivateUser(ctx context.Context, targetID uuid.UUID, actorID uuid.UUID) error {
	if targetID == actorID {
		return domain.NewBizError(domain.ErrInvalidInput, "không thể tự vô hiệu hóa tài khoản của chính bạn")
	}
	return s.st.deactivateUser(ctx, targetID)
}

func (s *service) mapUserToDetail(u user) UserDetail {
	return UserDetail{
		ID:        u.ID,
		Username:  u.Username,
		Role:      u.Role,
		FullName:  u.FullName,
		Email:     u.Email,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

func (s *service) validateRole(role string) error {
	r := auth.Role(role)
	if r == auth.RoleAdmin || r == auth.RoleAccountant || r == auth.RolePlanner || 
       r == auth.RoleWarehouse || r == auth.RoleCNC || r == auth.RoleCNCManager || r == auth.RoleForeman {
		return nil
	}
	return domain.NewBizError(domain.ErrInvalidInput, "invalid role: "+role)
}
