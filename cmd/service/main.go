// Command service runs the API, explicit migrations or build-identity inspection.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"boilerplate/internal/app"
	"boilerplate/internal/buildinfo"
	"boilerplate/internal/config"
	"boilerplate/internal/platform/database"
	"boilerplate/migrations"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	environment := os.Getenv("APP_ENV")
	if environment == "" {
		environment = "local"
	}

	env := flag.String("env", environment, "deployment label (local/test enable local conveniences)")
	dir := flag.String("config-dir", "configs", "optional YAML directory for local/test only; empty disables files")
	migrate := flag.Bool("migrate", false, "apply PostgreSQL migrations and exit")
	showVersion := flag.Bool("version", false, "print build metadata as JSON and exit")
	flag.Parse()

	metadata := buildinfo.Current()
	if *showVersion {
		return json.NewEncoder(os.Stdout).Encode(metadata)
	}

	c, err := config.Load(*dir, *env)
	if err != nil {
		return err
	}
	if *migrate {
		return runMigrations(c)
	}

	// Framework debug output bypasses structured logging; app logging levels remain configurable.
	gin.SetMode(gin.ReleaseMode)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := app.NewRuntime(ctx, c, metadata.Version)
	if err != nil {
		return err
	}

	// Reload is process-owned; static resource changes are rejected by Runtime.Reload.
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	defer signal.Stop(reload)
	reloadDone := make(chan struct{})
	defer close(reloadDone)
	go func() {
		for {
			select {
			case <-reloadDone:
				return
			case <-ctx.Done():
				return
			case <-reload:
				next, loadErr := config.Load(*dir, *env)
				if loadErr != nil {
					runtime.Logger.Warn("configuration reload rejected", "reason", "invalid configuration")
					continue
				}
				if reloadErr := runtime.Reload(next); reloadErr != nil {
					runtime.Logger.Warn("configuration reload rejected", "reason", reloadErr.Error())
				}
			}
		}
	}()

	runtime.Logger.Info("build metadata", "build", metadata)
	runtime.Logger.Info("configuration loaded", "config", c.Redacted())
	return runtime.Run(ctx)
}

func runMigrations(c config.Config) (err error) {
	if c.Database.Driver != "postgres" {
		return errors.New("migrations require database.driver=postgres")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Database.MigrationTimeout)
	defer cancel()
	pool, err := database.Open(ctx, c.Database)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), c.Shutdown.CleanupTimeout)
		defer stop()
		err = errors.Join(err, database.Close(cleanup, pool))
	}()
	return migrations.Apply(ctx, pool)
}
