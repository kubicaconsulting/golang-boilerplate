// Package memory provides concurrency-safe, non-durable local persistence.
package memory

import (
	"context"
	"sort"
	"sync"

	userdomain "boilerplate/internal/users/domain"
)

// Repository stores users in memory for local/test execution only.
type Repository struct {
	mu     sync.RWMutex
	users  map[string]userdomain.User
	emails map[string]string
}

// NewRepository constructs an empty, concurrency-safe local repository.
func NewRepository() *Repository {
	return &Repository{users: map[string]userdomain.User{}, emails: map[string]string{}}
}

// Create inserts a user atomically while enforcing email uniqueness.
func (r *Repository) Create(ctx context.Context, user userdomain.User) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.users[user.ID]; ok {
		return userdomain.ErrConflict
	}
	if _, ok := r.emails[user.Email]; ok {
		return userdomain.ErrConflict
	}

	r.users[user.ID] = user
	r.emails[user.Email] = user.ID
	return nil
}

// FindByID returns the user or the domain not-found error.
func (r *Repository) FindByID(ctx context.Context, id string) (userdomain.User, error) {
	if err := ctx.Err(); err != nil {
		return userdomain.User{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[id]
	if !ok {
		return userdomain.User{}, userdomain.ErrNotFound
	}

	return user, nil
}

// List returns a bounded page in deterministic creation/ID order.
func (r *Repository) List(ctx context.Context, limit, offset int) ([]userdomain.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]userdomain.User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, user)
	}

	sort.Slice(users, func(i, j int) bool {
		if users[i].CreatedAt.Equal(users[j].CreatedAt) {
			return users[i].ID < users[j].ID
		}
		return users[i].CreatedAt.Before(users[j].CreatedAt)
	})

	if offset >= len(users) {
		return []userdomain.User{}, nil
	}
	end := min(offset+limit, len(users))
	return users[offset:end], nil
}
