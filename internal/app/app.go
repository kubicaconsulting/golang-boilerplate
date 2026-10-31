// Package app is the composition root: it alone selects and owns adapters.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/auth"
	"boilerplate/internal/platform/database"
	"boilerplate/internal/platform/featureflags"
	"boilerplate/internal/platform/httpserver"
	"boilerplate/internal/platform/observability"
	userhttp "boilerplate/internal/users/adapters/http"
	"boilerplate/internal/users/adapters/memory"
	"boilerplate/internal/users/adapters/postgres"
	userapp "boilerplate/internal/users/application"
)

// Runtime owns the assembled service and its process-lifetime resources.
type Runtime struct {
	Config    config.Config
	HTTP      *httpserver.Server
	Logger    *slog.Logger
	resources Resources
	telemetry *observability.Runtime
	flags     *featureflags.Provider
	mu        sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

// NewRuntime validates configuration and assembles adapters, unwinding partial startup on failure.
//
//nolint:funlen // Keep resource ownership, initialization events and failure cleanup visible in construction order.
func NewRuntime(ctx context.Context, cfg config.Config, version string) (_ *Runtime, err error) {
	started := time.Now()
	phase := started
	if err = cfg.Validate(); err != nil {
		return nil, err
	}

	// Users need durable storage in deployments; stateless services can remove this guard.
	if !config.IsLocal(cfg.Service.Environment) && cfg.Database.Driver == "memory" {
		return nil, errors.New("users feature requires durable persistence outside local/test")
	}

	if err = ctx.Err(); err != nil {
		return nil, err
	}

	runtime := &Runtime{Config: cfg}
	defer func() {
		if err != nil {
			err = errors.Join(err, runtime.Close(context.Background()))
		}
	}()

	runtime.telemetry, err = observability.NewRuntime(ctx, cfg, version)
	if err != nil {
		runtime.telemetry = nil // The telemetry constructor already unwinds partial initialization.
		return nil, err
	}
	logger := runtime.telemetry.Logger
	runtime.Logger = logger.With("component", "Application")
	logger.InfoContext(ctx, "observability initialized", "component", "Observability", "duration", time.Since(phase))
	phase = time.Now()

	var repository userapp.Repository = memory.NewRepository()
	var checkDatabase func(context.Context) error
	if cfg.Database.Driver == "postgres" {
		pool, openErr := database.Open(ctx, cfg.Database)
		if openErr != nil {
			return nil, openErr
		}
		runtime.resources.Add("postgres", func(ctx context.Context) error { return database.Close(ctx, pool) })
		repository = postgres.NewRepository(pool)
		checkDatabase = pool.Ping
	}

	logger.InfoContext(ctx, "persistence initialized", "component", "Database",
		"driver", cfg.Database.Driver, "duration", time.Since(phase))
	phase = time.Now()

	// Authentication survives request cancellation and closes during resource cleanup.
	authenticator, authErr := auth.NewAuthenticator(context.Background(), cfg.Auth)
	if authErr != nil {
		return nil, authErr
	}
	runtime.resources.Add("authentication", authenticator.Close)
	logger.InfoContext(ctx, "authentication initialized", "component", "Authentication", "duration", time.Since(phase))
	phase = time.Now()

	runtime.flags, err = featureflags.NewProvider(cfg.Flags, logger.With("component", "FeatureFlags"))
	if err != nil {
		return nil, err
	}
	runtime.resources.Add("feature flags", runtime.flags.Close)
	logger.InfoContext(ctx, "feature flags initialized", "component", "FeatureFlags",
		"provider", cfg.Flags.Provider, "duration", time.Since(phase))
	phase = time.Now()

	usersService := userapp.NewService(repository, runtime.flags)
	usersHandler := userhttp.NewHandler(usersService, logger.With("component", "Users"))
	logger.InfoContext(ctx, "users feature initialized", "component", "Users", "duration", time.Since(phase))
	phase = time.Now()
	runtime.HTTP = httpserver.NewServer(
		cfg.HTTP, usersHandler, authenticator, logger.With("component", "HTTP"), checkDatabase,
	)
	logger.InfoContext(ctx, "HTTP routes registered", "component", "HTTP", "duration", time.Since(phase))
	runtime.Logger.InfoContext(ctx, "application initialized", "duration", time.Since(started))
	return runtime, nil
}

// Close reserves independent infrastructure and telemetry budgets. Its caller
// may impose a stricter deadline; Run deliberately does not pass the drain context.
func (r *Runtime) Close(ctx context.Context) error {
	if r.HTTP != nil {
		r.HTTP.SetReady(false)
	}

	r.closeOnce.Do(func() {
		// Keep telemetry alive while other resources emit their final events.
		cleanup, cancel := context.WithTimeout(ctx, r.Config.Shutdown.CleanupTimeout)
		r.closeErr = r.resources.Close(cleanup)
		cancel()
		if r.closeErr != nil && r.Logger != nil {
			r.Logger.ErrorContext(ctx, "resource cleanup incomplete")
		}

		// A slow closer must not consume the reserved telemetry flush budget.
		if r.telemetry != nil {
			flush, stop := context.WithTimeout(ctx, r.Config.Shutdown.TelemetryTimeout)
			r.closeErr = errors.Join(r.closeErr, r.telemetry.Close(flush))
			stop()
		}
	})

	return r.closeErr
}

// Reload applies validated dynamic settings and rejects changes requiring resource replacement.
func (r *Runtime) Reload(c config.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := c.Validate(); err != nil {
		return err
	}
	if !r.Config.SameStatic(c) {
		return errors.New("reload rejected: static configuration changed; restart required")
	}

	var level slog.Level
	_ = level.UnmarshalText([]byte(c.Logging.Level))

	r.telemetry.Level.Set(level)
	r.flags.Update(c.Flags.Values)
	r.Logger.Info("runtime configuration reloaded")
	return nil
}

// Run binds before becoming ready, drains HTTP, then closes infrastructure.
func (r *Runtime) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              r.Config.HTTP.Address,
		Handler:           r.HTTP.Handler,
		ReadHeaderTimeout: r.Config.HTTP.ReadHeaderTimeout,
		ReadTimeout:       r.Config.HTTP.ReadTimeout,
		WriteTimeout:      r.Config.HTTP.WriteTimeout,
		IdleTimeout:       r.Config.HTTP.IdleTimeout,
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", server.Addr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen: %w", err), r.Close(context.Background()))
	}
	return r.serve(ctx, server, listener)
}

func (r *Runtime) serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	var err error
	r.HTTP.SetReady(true)
	r.Logger.InfoContext(ctx, "service ready", "address", listener.Addr().String())

	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	select {
	case <-ctx.Done():
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}

	// Stop accepting traffic before draining requests that still own dependencies.
	r.HTTP.SetReady(false)
	shutdown, cancel := context.WithTimeout(context.Background(), r.Config.Shutdown.DrainTimeout)
	defer cancel()

	drainErr := server.Shutdown(shutdown)
	if drainErr != nil {
		drainErr = errors.Join(drainErr, server.Close())
	}

	return errors.Join(err, drainErr, r.Close(context.Background()))
}
