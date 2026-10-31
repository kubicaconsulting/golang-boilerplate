// Package featureflags isolates provider SDKs from application use cases.
package featureflags

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync/atomic"

	"boilerplate/internal/config"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
)

type snapshot struct{ values map[string]bool }

// Provider evaluates flags using immutable local defaults and an optional SDK client.
type Provider struct {
	local  atomic.Pointer[snapshot]
	client *ld.LDClient
	logger *slog.Logger
}

// NewProvider initializes flag evaluation and preserves fallbacks if the remote provider is unavailable.
func NewProvider(c config.Flags, logger *slog.Logger) (*Provider, error) {
	p := &Provider{logger: logger}
	p.Update(c.Values)

	if c.Provider == "launchdarkly" {
		logs := ldlog.NewDefaultLoggers()
		logs.SetMinLevel(ldlog.Debug)

		// None disables logging and is not an emitted severity.
		//exhaustive:ignore
		levels := map[ldlog.LogLevel]slog.Level{
			ldlog.Debug: slog.LevelDebug,
			ldlog.Info:  slog.LevelInfo,
			ldlog.Warn:  slog.LevelWarn,
			ldlog.Error: slog.LevelError,
		}
		for level, threshold := range levels {
			logs.SetBaseLoggerForLevel(level, sdkLogger{logger: logger, level: threshold})
		}

		client, err := ld.MakeCustomClient(
			c.SDKKey,
			ld.Config{Logging: ldcomponents.Logging().Loggers(logs).LogContextKeyInErrors(false)},
			c.InitTimeout,
		)
		p.client = client
		if err != nil {
			logger.Warn("LaunchDarkly not initialized; using explicit fallbacks", "error_type", "initialization")
		}
	}
	return p, nil
}

// Update atomically replaces local fallback values with a private copy.
func (p *Provider) Update(values map[string]bool) {
	// Publish a private snapshot so caller mutations cannot race with evaluations.
	copied := make(map[string]bool, len(values))
	maps.Copy(copied, values)

	p.local.Store(&snapshot{values: copied})
}

// Bool evaluates a flag for a subject, returning the explicit fallback on provider failure.
func (p *Provider) Bool(ctx context.Context, key, subject string, fallback bool) bool {
	if b, ok := p.local.Load().values[key]; ok {
		fallback = b
	}
	if p.client == nil {
		return fallback
	}
	if subject == "" {
		subject = "anonymous"
	}

	value, err := p.client.BoolVariationCtx(ctx, key, ldcontext.New(subject), fallback)
	if err != nil {
		p.logger.WarnContext(ctx, "flag evaluation used fallback", "flag", key)
	}
	return value
}

// Close releases the optional provider client.
func (p *Provider) Close(context.Context) error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// sdkLogger preserves provider severity while using the configured structured sink.
type sdkLogger struct {
	logger *slog.Logger
	level  slog.Level
}

func (l sdkLogger) Println(values ...any) {
	l.logger.Log(context.Background(), l.level, fmt.Sprint(values...), "provider", "launchdarkly")
}

func (l sdkLogger) Printf(format string, values ...any) {
	l.logger.Log(context.Background(), l.level, fmt.Sprintf(format, values...), "provider", "launchdarkly")
}
