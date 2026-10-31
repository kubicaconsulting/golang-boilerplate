// Package database owns PostgreSQL resources, not feature queries.
package database

import (
	"context"
	"errors"

	"boilerplate/internal/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type tracer struct{}

//nolint:spancheck // pgx owns the callback pair; TraceQueryEnd ends the span carried in the returned context.
func (tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	ctx, _ = otel.Tracer("database").Start(
		ctx,
		"postgres.query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system.name", "postgresql")),
	)
	return ctx
}

func (tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	s := trace.SpanFromContext(ctx)
	if d.Err != nil {
		s.SetStatus(codes.Error, "database operation failed")
	}
	s.End()
}

// Open constructs and pings a PostgreSQL pool, closing it if initialization fails.
func Open(ctx context.Context, c config.Database) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(c.DSN)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL DSN (redacted)")
	}

	cfg.MaxConns = c.MaxConns
	cfg.MinConns = c.MinConns
	cfg.MaxConnLifetime = c.MaxConnLifetime
	cfg.MaxConnIdleTime = c.MaxConnIdleTime
	cfg.ConnConfig.ConnectTimeout = c.ConnectTimeout
	cfg.ConnConfig.Tracer = tracer{}

	startup, cancel := context.WithTimeout(ctx, c.ConnectTimeout)
	defer cancel()

	p, err := pgxpool.NewWithConfig(startup, cfg)
	if err != nil {
		return nil, errors.New("initialize PostgreSQL pool")
	}
	if err = p.Ping(startup); err != nil {
		p.Close()
		return nil, errors.New("PostgreSQL startup connection failed")
	}
	return p, nil
}

// Close bounds waiting for borrowers; application draining must happen first.
func Close(ctx context.Context, p *pgxpool.Pool) error {
	done := make(chan struct{})
	go func() {
		p.Close()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
