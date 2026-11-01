// Package featureflags isolates provider SDKs from application use cases.
package featureflags

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"

	"boilerplate/internal/config"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldfiledata"
)

const fileProvider = "file"

type snapshot struct{ values map[string]json.RawMessage }

// Provider evaluates flags using immutable local defaults and an optional SDK client.
type Provider struct {
	local  atomic.Pointer[snapshot]
	client *ld.LDClient
	logger *slog.Logger
}

// NewProvider initializes flag evaluation and preserves fallbacks if the remote provider is unavailable.
func NewProvider(c config.Flags, logger *slog.Logger) (*Provider, error) {
	p := &Provider{logger: logger}
	if err := p.Update(c.Values); err != nil {
		return nil, err
	}
	if c.Provider == "local" {
		return p, nil
	}
	if c.Provider != "launchdarkly" && c.Provider != fileProvider {
		return nil, errors.New("unsupported flags.provider")
	}

	sdkConfig := ld.Config{Logging: sdkLogging(logger)}
	sdkKey := c.SDKKey
	if c.Provider == fileProvider {
		if len(c.Files) == 0 {
			return nil, errors.New("file flags provider requires flags.files")
		}
		sdkConfig.DataSource = ldfiledata.DataSource().FilePaths(c.Files...)
		sdkConfig.Events = ldcomponents.NoEvents()
		sdkConfig.DiagnosticOptOut = true
		sdkKey = "file-data"
	}
	client, err := ld.MakeCustomClient(sdkKey, sdkConfig, c.InitTimeout)
	p.client = client
	if c.Provider == fileProvider && (err != nil || client == nil || !client.Initialized()) {
		if client != nil {
			_ = client.Close()
		}
		return nil, errors.New("feature flag files could not be initialized")
	}
	if err != nil {
		logger.Warn("LaunchDarkly not initialized; using explicit fallbacks", "error_type", "initialization")
	}
	return p, nil
}

// Update atomically replaces local fallback values with a private copy.
func (p *Provider) Update(values map[string]any) error {
	// Serialize before publishing: nested caller-owned maps and lists must not alias the snapshot.
	encoded, err := json.Marshal(values)
	if err != nil {
		return errors.New("flag overrides must contain valid JSON values")
	}
	var copied map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &copied); err != nil {
		return errors.New("flag overrides could not be decoded")
	}
	p.local.Store(&snapshot{values: copied})
	return nil
}

// Bool evaluates a boolean flag, retaining the declared fallback on type or provider errors.
func (p *Provider) Bool(ctx context.Context, key, subject string, fallback bool) bool {
	fallback = localFallback(ctx, p, key, fallback)
	if p.client == nil {
		return fallback
	}
	value, err := p.client.BoolVariationCtx(ctx, key, evaluationContext(subject), fallback)
	if err != nil {
		p.warnFallback(ctx, key)
	}
	return value
}

// String evaluates a string flag, retaining the declared fallback on type or provider errors.
func (p *Provider) String(ctx context.Context, key, subject, fallback string) string {
	fallback = localFallback(ctx, p, key, fallback)
	if p.client == nil {
		return fallback
	}
	value, err := p.client.StringVariationCtx(ctx, key, evaluationContext(subject), fallback)
	if err != nil {
		p.warnFallback(ctx, key)
	}
	return value
}

// Enum accepts only declared string alternatives. An invalid fallback is a definition error.
func (p *Provider) Enum(ctx context.Context, key, subject, fallback string, allowed ...string) (string, error) {
	if !slices.Contains(allowed, fallback) {
		return "", errors.New("enum flag fallback must be one of its allowed values")
	}
	value := p.String(ctx, key, subject, fallback)
	if !slices.Contains(allowed, value) {
		p.warnFallback(ctx, key)
		return fallback, nil
	}
	return value, nil
}

// JSON evaluates any JSON value and returns an owned copy. Invalid fallback JSON is a definition error.
// Application code must decode and validate the resulting business configuration before using it.
func (p *Provider) JSON(ctx context.Context, key, subject string, fallback json.RawMessage) (json.RawMessage, error) {
	if !json.Valid(fallback) {
		return nil, errors.New("JSON flag fallback must be valid JSON")
	}
	if value, ok := p.local.Load().values[key]; ok {
		fallback = value
	}
	if p.client == nil {
		return bytes.Clone(fallback), nil
	}
	value, err := p.client.JSONVariationCtx(ctx, key, evaluationContext(subject), ldvalue.Raw(fallback))
	if err != nil {
		p.warnFallback(ctx, key)
	}
	return bytes.Clone(value.AsRaw()), nil
}

// StringList evaluates a JSON array of strings. Other JSON shapes use a fresh copy of the fallback.
func (p *Provider) StringList(ctx context.Context, key, subject string, fallback []string) []string {
	// A string slice has no unsupported JSON values.
	encoded, _ := json.Marshal(fallback)
	raw, err := p.JSON(ctx, key, subject, encoded)
	var values []any
	if err != nil || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &values) != nil {
		p.warnFallback(ctx, key)
		return slices.Clone(fallback)
	}
	result := make([]string, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			p.warnFallback(ctx, key)
			return slices.Clone(fallback)
		}
		result[i] = text
	}
	return result
}

func localFallback[T bool | string](ctx context.Context, p *Provider, key string, fallback T) T {
	if raw, ok := p.local.Load().values[key]; ok {
		var value T
		if !bytes.Equal(raw, []byte("null")) && json.Unmarshal(raw, &value) == nil {
			return value
		}
		p.warnFallback(ctx, key)
	}
	return fallback
}

func evaluationContext(subject string) ldcontext.Context {
	if subject == "" {
		subject = "anonymous"
	}
	return ldcontext.New(subject)
}

func (p *Provider) warnFallback(ctx context.Context, key string) {
	p.logger.WarnContext(ctx, "flag evaluation used fallback", "flag", key)
}

func sdkLogging(logger *slog.Logger) *ldcomponents.LoggingConfigurationBuilder {
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
	return ldcomponents.Logging().Loggers(logs).LogContextKeyInErrors(false)
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
