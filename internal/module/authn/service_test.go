package authn

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) selectUserByUsername(ctx context.Context, username string) (user, error) {
	args := m.Called(ctx, username)
	return args.Get(0).(user), args.Error(1)
}

func (m *mockStore) selectUserByID(ctx context.Context, id uuid.UUID) (user, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(user), args.Error(1)
}

func (m *mockStore) selectUsers(ctx context.Context, params ListUsersParams) ([]user, int, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]user), args.Int(1), args.Error(2)
}

func (m *mockStore) insertUser(ctx context.Context, u user) (user, error) {
	args := m.Called(ctx, u)
	return args.Get(0).(user), args.Error(1)
}

func (m *mockStore) updateUser(ctx context.Context, u user) (user, error) {
	args := m.Called(ctx, u)
	return args.Get(0).(user), args.Error(1)
}

func (m *mockStore) updatePassword(ctx context.Context, userID uuid.UUID, hash string) error {
	args := m.Called(ctx, userID, hash)
	return args.Error(0)
}

func (m *mockStore) deactivateUser(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestService_CreateUser(t *testing.T) {
	st := new(mockStore)
	svc := NewService(st, "secret")
	ctx := context.Background()
	creatorID := uuid.New()

	t.Run("success", func(t *testing.T) {
		in := CreateUserInput{
			Username: "testuser",
			Password: "password123",
			Role:     string(auth.RolePlanner),
			FullName: "Test User",
			Email:    "test@vmarble.com",
		}

		st.On("insertUser", ctx, mock.MatchedBy(func(u user) bool {
			return u.Username == "testuser" && u.Role == string(auth.RolePlanner)
		})).Return(user{
			ID:       uuid.New(),
			Username: in.Username,
			Role:     in.Role,
			IsActive: true,
		}, nil).Once()

		result, err := svc.CreateUser(ctx, creatorID, in)
		assert.NoError(t, err)
		assert.Equal(t, "testuser", result.Username)
		st.AssertExpectations(t)
	})

	t.Run("invalid role", func(t *testing.T) {
		in := CreateUserInput{
			Username: "testuser",
			Password: "password123",
			Role:     "invalid_role",
		}

		_, err := svc.CreateUser(ctx, creatorID, in)
		assert.Error(t, err)
		bizErr, ok := err.(*domain.BizError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrInvalidInput, bizErr.Sentinel)
	})
}

func TestService_DeactivateUser(t *testing.T) {
	st := new(mockStore)
	svc := NewService(st, "secret")
	ctx := context.Background()
	adminID := uuid.New()
	targetID := uuid.New()

	t.Run("success", func(t *testing.T) {
		st.On("deactivateUser", ctx, targetID).Return(nil).Once()

		err := svc.DeactivateUser(ctx, targetID, adminID)
		assert.NoError(t, err)
		st.AssertExpectations(t)
	})

	t.Run("cannot deactivate self", func(t *testing.T) {
		err := svc.DeactivateUser(ctx, adminID, adminID)
		assert.Error(t, err)
		bizErr, ok := err.(*domain.BizError)
		assert.True(t, ok)
		assert.Equal(t, domain.ErrInvalidInput, bizErr.Sentinel)
	})
}

func TestService_ListUsers(t *testing.T) {
	st := new(mockStore)
	svc := NewService(st, "secret")
	ctx := context.Background()

	t.Run("success with filters", func(t *testing.T) {
		params := ListUsersParams{
			Roles: []string{string(auth.RolePlanner)},
		}
		expectedUsers := []user{
			{ID: uuid.New(), Username: "planner1", Role: string(auth.RolePlanner), IsActive: true},
		}

		st.On("selectUsers", ctx, params).Return(expectedUsers, 1, nil).Once()

		result, err := svc.ListUsers(ctx, params)
		assert.NoError(t, err)
		assert.Equal(t, 1, result.TotalItems)
		assert.Equal(t, "planner1", result.Items[0].Username)
		st.AssertExpectations(t)
	})
}
