// Package application implements users use cases and defines their required ports.
package application

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"time"

	"boilerplate/internal/platform/featureflags/definitions"
	userdomain "boilerplate/internal/users/domain"
)

var (
	// ErrForbidden indicates that the actor lacks the required use-case permission.
	ErrForbidden = errors.New("permission denied")
	// ErrDisabled indicates that the users-list feature is unavailable.
	ErrDisabled = errors.New("operation disabled")
)

// Actor carries verified identity and permissions independently of transport authentication.
type Actor struct {
	Subject string
	Scopes  []string
}

// Can reports whether the actor has the requested scope.
func (actor Actor) Can(scope string) bool {
	return slices.Contains(actor.Scopes, scope)
}

// Repository declares the persistence operations consumed by users use cases.
type Repository interface {
	Create(context.Context, userdomain.User) error
	FindByID(context.Context, string) (userdomain.User, error)
	List(context.Context, int, int) ([]userdomain.User, error)
}

// Flags declares the boolean evaluation consumed by users use cases.
type Flags interface {
	Bool(context.Context, string, string, bool) bool
}

// Service coordinates authorization, domain invariants and consumer-owned ports.
type Service struct {
	users Repository
	flags Flags
}

// NewService injects users persistence and flag evaluation without provider types.
func NewService(users Repository, flags Flags) *Service {
	return &Service{users: users, flags: flags}
}

// Create authorizes the actor, constructs a valid user and persists it.
func (s *Service) Create(ctx context.Context, actor Actor, name, email string) (userdomain.User, error) {
	if !actor.Can("users:write") {
		return userdomain.User{}, ErrForbidden
	}
	if err := ctx.Err(); err != nil {
		return userdomain.User{}, err
	}

	user, err := userdomain.NewUser(rand.Text(), name, email, time.Now())
	if err != nil {
		return userdomain.User{}, err
	}
	if err = s.users.Create(ctx, user); err != nil {
		return userdomain.User{}, err
	}

	return user, nil
}

// Get authorizes reads before loading the requested user.
func (s *Service) Get(ctx context.Context, actor Actor, id string) (userdomain.User, error) {
	if !actor.Can("users:read") {
		return userdomain.User{}, ErrForbidden
	}

	return s.users.FindByID(ctx, id)
}

// List authorizes reads and evaluates availability before requesting a page.
func (s *Service) List(ctx context.Context, actor Actor, limit, offset int) ([]userdomain.User, error) {
	if !actor.Can("users:read") {
		return nil, ErrForbidden
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, userdomain.ErrInvalid
	}
	if !s.flags.Bool(ctx, definitions.UsersListKey, actor.Subject, definitions.UsersListFallback) {
		return nil, ErrDisabled
	}

	return s.users.List(ctx, limit, offset)
}
