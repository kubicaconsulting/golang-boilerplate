// Package config loads immutable, validated configuration snapshots.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"boilerplate/internal/platform/featureflags/definitions"

	"go.yaml.in/yaml/v3"
)

const localEnvironment = "local"

// Config is a complete validated configuration snapshot; secrets must not be logged.
type Config struct {
	Service   Service   `yaml:"service"`
	HTTP      HTTP      `yaml:"http"`
	Database  Database  `yaml:"database"`
	Logging   Logging   `yaml:"logging"`
	Telemetry Telemetry `yaml:"telemetry"`
	Auth      Auth      `yaml:"auth"`
	Flags     Flags     `yaml:"flags"`
	Shutdown  Shutdown  `yaml:"shutdown"`
}

// Shutdown reserves independent deadlines for draining, resource cleanup and telemetry.
type Shutdown struct {
	DrainTimeout     time.Duration `yaml:"drain_timeout"`
	CleanupTimeout   time.Duration `yaml:"cleanup_timeout"`
	TelemetryTimeout time.Duration `yaml:"telemetry_timeout"`
}

// Service identifies a service and its deployment label.
type Service struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"-"`
}

// HTTP defines listener limits, timeouts and documentation visibility.
type HTTP struct {
	Address           string        `yaml:"address"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	MaxBodyBytes      int64         `yaml:"max_body_bytes"`
	Docs              bool          `yaml:"docs"`
}

// Database selects persistence and bounds connection pools and migration execution.
type Database struct {
	Driver           string        `yaml:"driver"`
	DSN              string        `yaml:"dsn"`
	MaxConns         int32         `yaml:"max_conns"`
	MinConns         int32         `yaml:"min_conns"`
	MaxConnLifetime  time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime  time.Duration `yaml:"max_conn_idle_time"`
	ConnectTimeout   time.Duration `yaml:"connect_timeout"`
	MigrationTimeout time.Duration `yaml:"migration_timeout"`
}

// Logging sets the structured sink format and reloadable severity threshold.
type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Color  string `yaml:"color"`
}

// Telemetry controls optional OTLP exporters, sampling and export deadlines.
type Telemetry struct {
	Enabled       bool          `yaml:"enabled"`
	SampleRatio   float64       `yaml:"sample_ratio"`
	ExportTimeout time.Duration `yaml:"export_timeout"`
}

// Auth selects the accepted machine and bearer credential strategies.
type Auth struct {
	APIKeys []APIKey `yaml:"api_keys"`
	JWT     JWT      `yaml:"jwt"`
}

// APIKey stores a credential digest and its trusted subject/scopes, never the raw key.
type APIKey struct {
	ID      string   `yaml:"id" json:"id"`
	SHA256  string   `yaml:"sha256" json:"sha256"`
	Subject string   `yaml:"subject" json:"subject"`
	Scopes  []string `yaml:"scopes" json:"scopes"`
}

// JWT defines the trusted issuer, audience and verification-key source.
type JWT struct {
	Enabled      bool   `yaml:"enabled"`
	Issuer       string `yaml:"issuer"`
	Audience     string `yaml:"audience"`
	JWKSURL      string `yaml:"jwks_url"`
	PublicKeyPEM string `yaml:"public_key_pem"`
}

// Flags selects the evaluation provider and JSON-compatible local fallback overrides.
type Flags struct {
	Provider    string         `yaml:"provider"`
	SDKKey      string         `yaml:"sdk_key"`
	InitTimeout time.Duration  `yaml:"init_timeout"`
	Values      map[string]any `yaml:"values"`
	Files       []string       `yaml:"files"`
}

// Defaults returns fresh local-development defaults without authentication credentials.
//
//nolint:mnd // Typed default fields are the central declaration of operational policy values.
func Defaults() Config {
	return Config{
		Service: Service{Name: "microservice", Environment: localEnvironment},
		HTTP: HTTP{
			Address:           "127.0.0.1:8080",
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxBodyBytes:      1 << 20,
			Docs:              true,
		},
		Database: Database{
			Driver:           "memory",
			MaxConns:         10,
			MaxConnLifetime:  time.Hour,
			MaxConnIdleTime:  15 * time.Minute,
			ConnectTimeout:   5 * time.Second,
			MigrationTimeout: 30 * time.Second,
		},
		Logging:   Logging{Level: "info", Format: "json", Color: "auto"},
		Telemetry: Telemetry{SampleRatio: 0.1, ExportTimeout: 5 * time.Second},
		Flags: Flags{
			Provider:    "local",
			InitTimeout: 5 * time.Second,
			Values:      map[string]any{definitions.UsersListKey: definitions.UsersListFallback},
		},
		Shutdown: Shutdown{
			DrainTimeout:     10 * time.Second,
			CleanupTimeout:   5 * time.Second,
			TelemetryTimeout: 5 * time.Second,
		},
	}
}

// IsLocal permits local conveniences only in explicit local/test execution modes.
// Other environment names are deployment labels, not configuration-file profiles.
func IsLocal(environment string) bool {
	return environment == localEnvironment || environment == "test"
}

// Load uses defaults plus APP_* values. Optional YAML is read only in local/test
// when dir is nonempty. Deployed processes never depend on filesystem config.
func Load(dir, environment string) (Config, error) {
	c := Defaults()
	if environment == "" {
		environment = localEnvironment
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(environment) {
		return c, errors.New("environment must be a lowercase deployment label (letters, digits, hyphens)")
	}
	c.Service.Environment = environment
	if !IsLocal(environment) {
		c.HTTP.Address = ":8080"
		c.HTTP.Docs = false
		c.Database.Driver = ""
	}
	if IsLocal(environment) && dir != "" {
		for _, name := range []string{"base.yaml", environment + ".yaml"} {
			if err := loadYAML(filepath.Join(dir, name), &c); err != nil {
				return c, err
			}
		}
	}
	if err := overlay(reflect.ValueOf(&c).Elem(), "APP"); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func loadYAML(path string, c *Config) error {
	// #nosec G304 -- The operator explicitly selects local/test config files; deployments never read files.
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	err = dec.Decode(c)
	if err == nil {
		var extra any
		if next := dec.Decode(&extra); !errors.Is(next, io.EOF) {
			err = errors.New("configuration must contain exactly one YAML document")
		}
	}

	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("decode local config: %w", err)
	}
	return closeErr
}

func overlay(v reflect.Value, prefix string) error {
	t := v.Type()
	for i := range v.NumField() {
		f := v.Field(i)
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		key := prefix + "_" + strings.ToUpper(tag)
		if f.Kind() == reflect.Struct {
			if err := overlay(f, key); err != nil {
				return err
			}
			continue
		}

		raw, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		err := setEnvironmentValue(f, raw)
		if err != nil {
			return fmt.Errorf("invalid %s (value redacted)", key)
		}
	}
	return nil
}

// setEnvironmentValue handles scalars explicitly; structured values replace the snapshot field.
func setEnvironmentValue(field reflect.Value, raw string) error {
	if field.Type() == reflect.TypeFor[time.Duration]() {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		field.SetInt(int64(value))
		return nil
	}
	// Non-scalar kinds deliberately use the generic JSON replacement path.
	//exhaustive:ignore
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(value)
	case reflect.Int, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(value)
	case reflect.Float64:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		field.SetFloat(value)
	default:
		replacement := reflect.New(field.Type())
		if err := json.Unmarshal([]byte(raw), replacement.Interface()); err != nil {
			return err
		}
		field.Set(replacement.Elem())
	}
	return nil
}

// Validate checks the complete candidate before startup or reload.
func (c Config) Validate() error {
	for _, validate := range []func() error{
		c.validateLogging,
		c.validateRuntime,
		c.validateDatabase,
		c.validateAPIKeys,
		c.validateAuthentication,
		c.validateFlags,
	} {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) validateLogging() error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(c.Logging.Level)); err != nil {
		return errors.New("logging.level must be debug, info, warn or error")
	}
	if c.Logging.Level != "debug" && c.Logging.Level != "info" && c.Logging.Level != "warn" && c.Logging.Level != "error" {
		return errors.New("unsupported logging.level")
	}
	if c.Logging.Format != "json" && c.Logging.Format != "text" && c.Logging.Format != "console" {
		return errors.New("logging.format must be json, text or console")
	}
	if c.Logging.Color != "auto" && c.Logging.Color != "never" {
		return errors.New("logging.color must be auto or never")
	}
	return nil
}

func (c Config) validateRuntime() error {
	if c.Service.Name == "" ||
		c.HTTP.Address == "" ||
		c.HTTP.MaxBodyBytes < 1 ||
		c.Shutdown.DrainTimeout <= 0 ||
		c.Shutdown.CleanupTimeout <= 0 ||
		c.Shutdown.TelemetryTimeout <= 0 {
		return errors.New("invalid service/http/shutdown configuration")
	}
	for _, d := range []time.Duration{
		c.HTTP.ReadHeaderTimeout,
		c.HTTP.ReadTimeout,
		c.HTTP.WriteTimeout,
		c.HTTP.IdleTimeout,
		c.Database.ConnectTimeout,
		c.Database.MigrationTimeout,
		c.Telemetry.ExportTimeout,
		c.Flags.InitTimeout,
	} {
		if d <= 0 {
			return errors.New("timeouts must be positive")
		}
	}
	if math.IsNaN(c.Telemetry.SampleRatio) ||
		math.IsInf(c.Telemetry.SampleRatio, 0) ||
		c.Telemetry.SampleRatio < 0 ||
		c.Telemetry.SampleRatio > 1 {
		return errors.New("telemetry.sample_ratio must be between 0 and 1")
	}
	return nil
}

func (c Config) validateDatabase() error {
	if c.Database.Driver != "memory" && c.Database.Driver != "postgres" {
		return errors.New("database.driver must be memory or postgres")
	}
	if c.Database.Driver == "postgres" {
		if c.Database.DSN == "" ||
			c.Database.MaxConns < 1 ||
			c.Database.MinConns < 0 ||
			c.Database.MinConns > c.Database.MaxConns ||
			c.Database.MaxConnLifetime <= 0 ||
			c.Database.MaxConnIdleTime <= 0 {
			return errors.New("invalid postgres connection configuration")
		}
	}
	return nil
}

func (c Config) validateAPIKeys() error {
	ids := map[string]bool{}
	hashes := map[string]bool{}
	local := sha256.Sum256([]byte("local-development-key"))
	for _, key := range c.Auth.APIKeys {
		b, err := hex.DecodeString(key.SHA256)
		if err != nil ||
			len(b) != 32 ||
			key.ID == "" ||
			key.Subject == "" ||
			len(key.Scopes) == 0 ||
			ids[key.ID] ||
			hashes[strings.ToLower(key.SHA256)] {
			return errors.New("invalid or duplicate API key configuration")
		}
		ids[key.ID] = true
		hashes[strings.ToLower(key.SHA256)] = true
		if !IsLocal(c.Service.Environment) && strings.EqualFold(key.SHA256, hex.EncodeToString(local[:])) {
			return errors.New("development API key is prohibited outside local/test")
		}
	}
	return nil
}

func (c Config) validateAuthentication() error {
	if c.Auth.JWT.Enabled {
		j := c.Auth.JWT
		if j.Issuer == "" || j.Audience == "" || (j.JWKSURL == "") == (j.PublicKeyPEM == "") {
			return errors.New("JWT requires issuer, audience and exactly one of jwks_url/public_key_pem")
		}
		if j.JWKSURL != "" {
			u, err := url.Parse(j.JWKSURL)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				return errors.New("JWKS URL must use HTTPS")
			}
		}
	}
	if len(c.Auth.APIKeys) == 0 && !c.Auth.JWT.Enabled {
		return errors.New("configure at least one authentication strategy")
	}
	return nil
}

func (c Config) validateFlags() error {
	if c.Flags.Provider != "local" && c.Flags.Provider != "launchdarkly" && c.Flags.Provider != "file" {
		return errors.New("unsupported flags.provider")
	}
	if c.Flags.Provider == "launchdarkly" && c.Flags.SDKKey == "" {
		return errors.New("LaunchDarkly SDK key required")
	}
	if c.Flags.Provider == "file" && len(c.Flags.Files) == 0 {
		return errors.New("file flags provider requires flags.files")
	}
	for _, file := range c.Flags.Files {
		if strings.TrimSpace(file) == "" {
			return errors.New("flags.files entries must not be empty")
		}
	}
	if _, err := json.Marshal(c.Flags.Values); err != nil {
		return errors.New("flags.values must contain valid JSON values")
	}
	return nil
}

// SameStatic rejects reloads that require resource replacement or credential rotation.
func (c Config) SameStatic(other Config) bool {
	c.Logging.Level = ""
	other.Logging.Level = ""
	c.Flags.Values = nil
	other.Flags.Values = nil
	return reflect.DeepEqual(c, other)
}

// Redacted returns an allowlisted operational summary, never provider credentials.
func (c Config) Redacted() map[string]any {
	return map[string]any{
		"service":           c.Service.Name,
		"environment":       c.Service.Environment,
		"address":           c.HTTP.Address,
		"database_driver":   c.Database.Driver,
		"telemetry_enabled": c.Telemetry.Enabled,
		"flags_provider":    c.Flags.Provider,
		"log_level":         c.Logging.Level,
	}
}
