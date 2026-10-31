package application_test

import (
	"context"
	"errors"
	"testing"

	"boilerplate/internal/users/adapters/memory"
	"boilerplate/internal/users/application"
	users "boilerplate/internal/users/domain"
)

type flags bool

func (f flags) Bool(context.Context, string, string, bool) bool { return bool(f) }

func TestUseCases(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	svc := application.NewService(repo, flags(true))
	writer := application.Actor{Subject: "admin", Scopes: []string{"users:write", "users:read"}}
	if _, err := svc.Create(ctx, application.Actor{}, "Ada", "a@example.com"); !errors.Is(err, application.ErrForbidden) {
		t.Fatal(err)
	}
	u, err := svc.Create(ctx, writer, "Ada", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, createErr := svc.Create(ctx, writer, "Ada", "A@example.com"); !errors.Is(createErr, users.ErrConflict) {
		t.Fatal(createErr)
	}
	got, err := svc.Get(ctx, writer, u.ID)
	if err != nil || got != u {
		t.Fatalf("get: %v %v", got, err)
	}
	if _, err = svc.List(ctx, writer, 101, 0); !errors.Is(err, users.ErrInvalid) {
		t.Fatal(err)
	}
	_, err = application.NewService(repo, flags(false)).List(ctx, writer, 20, 0)
	if !errors.Is(err, application.ErrDisabled) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = svc.Create(canceled, writer, "Ada", "b@example.com"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
