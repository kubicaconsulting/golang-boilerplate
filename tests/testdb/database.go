//go:build integration || e2e

// Package testdb provisions isolated databases on an externally managed test PostgreSQL server.
package testdb

import (
	"context"
	"crypto/rand"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// CreateDatabase creates a database for a scenario and registers its removal with t.Cleanup.
// adminDSN must be a PostgreSQL URL for a dedicated test server with CREATEDB permission.
// Register service and pool cleanup after this call so they close before the database is dropped.
func CreateDatabase(t *testing.T, adminDSN string) string {
	t.Helper()
	endpoint, err := url.Parse(adminDSN)
	if err != nil || (endpoint.Scheme != "postgres" && endpoint.Scheme != "postgresql") {
		t.Fatal("test database admin DSN must be a postgres:// or postgresql:// URL")
	}
	const timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL administrator: %v", err)
	}
	// Cleanup uses its own context: the test context is canceled before cleanup runs.
	name := "test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{name}.Sanitize()
	created := false
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), timeout)
		defer stop()
		defer func() {
			if closeErr := admin.Close(cleanup); closeErr != nil {
				t.Errorf("close test database administrator: %v", closeErr)
			}
		}()
		if !created {
			return
		}
		if _, dropErr := admin.Exec(cleanup, "DROP DATABASE "+identifier); dropErr != nil {
			t.Errorf("drop isolated test database %s: %v", name, dropErr)
		}
	})
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier+" TEMPLATE template0"); err != nil {
		t.Fatalf("create isolated test database: %v", err)
	}

	created = true

	endpoint.Path = "/" + name
	endpoint.RawPath = ""
	query := endpoint.Query()
	// A query parameter must not override the isolated database selected in the path.
	query.Del("database")
	query.Del("dbname")
	endpoint.RawQuery = query.Encode()
	return endpoint.String()
}
