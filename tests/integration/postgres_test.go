//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/database"
	"boilerplate/internal/users/adapters/postgres"
	users "boilerplate/internal/users/domain"
	"boilerplate/migrations"
	"boilerplate/tests/testdb"
)

func TestPostgresRepository(t *testing.T) {
	t.Parallel()
	dsn := testdb.CreateDatabase(t, os.Getenv("TEST_DATABASE_ADMIN_DSN"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := config.Defaults().Database
	c.Driver = "postgres"
	c.DSN = dsn
	c.MaxConns = 3
	c.MinConns = 1
	c.MaxConnLifetime = 2 * time.Minute
	c.MaxConnIdleTime = time.Minute
	p, err := database.Open(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if p.Config().MaxConns != 3 || p.Config().MinConns != 1 || p.Config().MaxConnLifetime != 2*time.Minute {
		t.Fatal("pool settings not applied")
	}
	if err = migrations.Apply(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(ctx, p); err != nil {
		t.Fatal("migrations not idempotent:", err)
	}
	repo := postgres.NewRepository(p)
	id := "integration-" + time.Now().Format("150405.000000000")
	u, err := users.NewUser(id, "Ada", id+"@example.com", time.Now().Truncate(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err = repo.Create(ctx, u); !errors.Is(err, users.ErrConflict) {
		t.Fatal("unique constraint:", err)
	}
	got, err := repo.FindByID(ctx, id)
	if err != nil || got.ID != u.ID || got.Email != u.Email || !got.CreatedAt.Equal(u.CreatedAt) {
		t.Fatalf("roundtrip %#v %v", got, err)
	}
	if _, err = repo.FindByID(ctx, "missing"); !errors.Is(err, users.ErrNotFound) {
		t.Fatal(err)
	}
	rows, err := repo.List(ctx, 100, 0)
	if err != nil || len(rows) != 1 {
		t.Fatal("list failed", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err = repo.List(canceled, 20, 0); err == nil {
		t.Fatal("cancellation ignored")
	}
	// A single INSERT is atomic; demonstrate multi-write rollback with the same schema.
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE users SET name='Changed' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindByID(ctx, id)
	if err != nil || got.Name != "Ada" {
		t.Fatal("rollback failed")
	}
}
