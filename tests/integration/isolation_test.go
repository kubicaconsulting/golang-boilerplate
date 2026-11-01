//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"boilerplate/tests/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestDatabaseIsolation(t *testing.T) {
	adminDSN := os.Getenv("TEST_DATABASE_ADMIN_DSN")
	require.NotEmpty(t, adminDSN, "TEST_DATABASE_ADMIN_DSN must target a dedicated test server")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	require.NoError(t, err)
	defer func() { require.NoError(t, admin.Close(ctx)) }()

	names := make(chan string, 2)
	// The group waits for parallel scenarios and their cleanup before checking database removal.
	t.Run("independent scenarios", func(t *testing.T) {
		for _, scenario := range []string{"first", "second"} {
			t.Run(scenario, func(t *testing.T) {
				t.Parallel()
				dsn := testdb.CreateDatabase(t, adminDSN)
				connection, connectErr := pgx.Connect(t.Context(), dsn)
				require.NoError(t, connectErr)
				t.Cleanup(func() {
					cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
					defer stop()
					require.NoError(t, connection.Close(cleanup))
				})
				var name string
				require.NoError(t, connection.QueryRow(t.Context(), "SELECT current_database()").Scan(&name))
				names <- name
				_, setupErr := connection.Exec(t.Context(),
					"CREATE TABLE records (id integer PRIMARY KEY); INSERT INTO records VALUES (1)")
				require.NoError(t, setupErr)
				var count int
				require.NoError(t, connection.QueryRow(t.Context(), "SELECT count(*) FROM records").Scan(&count))
				require.Equal(t, 1, count)
			})
		}
	})
	close(names)
	for name := range names {
		var exists bool
		require.NoError(t, admin.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists))
		require.False(t, exists, "scenario database should be removed: %s", name)
	}
}
