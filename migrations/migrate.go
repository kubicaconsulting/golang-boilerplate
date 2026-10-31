// Package migrations provides explicit, transactional, serialized schema upgrades.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeout = 5 * time.Second

//go:embed *.sql
var files embed.FS

// Apply serializes and applies checksum-verified transactional migrations.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// Transaction-scoped advisory lock serializes concurrent migration commands.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(731925034)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations ("+
		"name text PRIMARY KEY, checksum text NOT NULL, "+
		"applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err = applyFile(ctx, tx, entry.Name()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func applyFile(ctx context.Context, tx pgx.Tx, name string) error {
	data, err := files.ReadFile(name)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	var existing string
	err = tx.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name=$1", name).Scan(&existing)
	if err == nil {
		if existing != checksum {
			return fmt.Errorf("migration %s changed after application", name)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err = tx.Exec(ctx, string(data)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations (name,checksum) VALUES ($1,$2)", name, checksum); err != nil {
		return err
	}
	return nil
}
