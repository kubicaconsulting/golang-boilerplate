package application_test

import (
	"context"
	"errors"
	"testing"

	"boilerplate/internal/users/application"
	users "boilerplate/internal/users/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) (*application.Service, *MockRepository, *MockFlags) {
	t.Helper()
	repository := NewMockRepository(t)
	flags := NewMockFlags(t)
	return application.NewService(repository, flags), repository, flags
}

func TestCreateUser(t *testing.T) {
	service, repository, _ := newTestService(t)
	actor := application.Actor{Subject: "admin", Scopes: []string{"users:write"}}
	repository.EXPECT().Create(t.Context(), mock.MatchedBy(func(user users.User) bool {
		return user.ID != "" && user.Name == "Ada" && user.Email == "ada@example.com" && !user.CreatedAt.IsZero()
	})).Return(nil).Once()

	user, err := service.Create(t.Context(), actor, " Ada ", " ADA@example.com ")

	require.NoError(t, err)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "Ada", user.Name)
	assert.Equal(t, "ada@example.com", user.Email)
}

func TestCreateUserRejectsInvalidInput(t *testing.T) {
	service, _, _ := newTestService(t)
	actor := application.Actor{Subject: "admin", Scopes: []string{"users:write"}}

	_, err := service.Create(t.Context(), actor, "", "invalid")

	require.ErrorIs(t, err, users.ErrInvalid)
}

func TestCreateUserPropagatesStorageFailure(t *testing.T) {
	service, repository, _ := newTestService(t)
	actor := application.Actor{Subject: "admin", Scopes: []string{"users:write"}}
	unavailable := errors.New("storage unavailable")
	repository.EXPECT().Create(t.Context(), mock.Anything).Return(unavailable).Once()

	_, err := service.Create(t.Context(), actor, "Ada", "ada@example.com")

	require.ErrorIs(t, err, unavailable)
}

func TestCreateUserHonorsCancellation(t *testing.T) {
	service, _, _ := newTestService(t)
	actor := application.Actor{Subject: "admin", Scopes: []string{"users:write"}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := service.Create(ctx, actor, "Ada", "ada@example.com")

	require.ErrorIs(t, err, context.Canceled)
}

func TestGetUser(t *testing.T) {
	for _, test := range []struct {
		name string
		user users.User
		err  error
	}{
		{name: "found", user: users.User{ID: "user-1", Name: "Ada", Email: "ada@example.com"}},
		{name: "missing", err: users.ErrNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, repository, _ := newTestService(t)
			actor := application.Actor{Subject: "reader", Scopes: []string{"users:read"}}
			repository.EXPECT().FindByID(t.Context(), "user-1").Return(test.user, test.err).Once()

			user, err := service.Get(t.Context(), actor, "user-1")

			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.user, user)
		})
	}
}

func TestListUsers(t *testing.T) {
	service, repository, flags := newTestService(t)
	actor := application.Actor{Subject: "reader", Scopes: []string{"users:read"}}
	expected := []users.User{{ID: "user-1", Name: "Ada", Email: "ada@example.com"}}
	flags.EXPECT().Bool(t.Context(), "users-list", actor.Subject, true).Return(true).Once()
	repository.EXPECT().List(t.Context(), 20, 0).Return(expected, nil).Once()

	actual, err := service.List(t.Context(), actor, 20, 0)

	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestListUsersDisabled(t *testing.T) {
	service, _, flags := newTestService(t)
	actor := application.Actor{Subject: "reader", Scopes: []string{"users:read"}}
	flags.EXPECT().Bool(t.Context(), "users-list", actor.Subject, true).Return(false).Once()

	actual, err := service.List(t.Context(), actor, 20, 0)

	require.ErrorIs(t, err, application.ErrDisabled)
	assert.Nil(t, actual)
}

func TestListUsersRejectsInvalidPagination(t *testing.T) {
	for _, test := range []struct {
		name   string
		limit  int
		offset int
	}{
		{name: "zero limit", limit: 0},
		{name: "excessive limit", limit: 101},
		{name: "negative offset", limit: 20, offset: -1},
		{name: "excessive offset", limit: 20, offset: 100001},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, _ := newTestService(t)
			actor := application.Actor{Subject: "reader", Scopes: []string{"users:read"}}

			_, err := service.List(t.Context(), actor, test.limit, test.offset)

			require.ErrorIs(t, err, users.ErrInvalid)
		})
	}
}

func TestListUsersPropagatesStorageFailure(t *testing.T) {
	service, repository, flags := newTestService(t)
	actor := application.Actor{Subject: "reader", Scopes: []string{"users:read"}}
	unavailable := errors.New("storage unavailable")
	flags.EXPECT().Bool(t.Context(), "users-list", actor.Subject, true).Return(true).Once()
	repository.EXPECT().List(t.Context(), 20, 0).Return(nil, unavailable).Once()

	_, err := service.List(t.Context(), actor, 20, 0)

	require.ErrorIs(t, err, unavailable)
}

func TestUserPermissions(t *testing.T) {
	actor := application.Actor{Subject: "unprivileged"}
	for _, operation := range []string{"create", "get", "list"} {
		t.Run(operation, func(t *testing.T) {
			service, _, _ := newTestService(t)
			var err error
			switch operation {
			case "create":
				_, err = service.Create(t.Context(), actor, "Ada", "ada@example.com")
			case "get":
				_, err = service.Get(t.Context(), actor, "user-1")
			case "list":
				_, err = service.List(t.Context(), actor, 20, 0)
			}
			require.ErrorIs(t, err, application.ErrForbidden)
		})
	}
}
