package config_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"boilerplate/internal/config"
)

func dir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "configs")
}

func TestPrecedenceAndLists(t *testing.T) {
	t.Setenv("APP_LOGGING_LEVEL", "warn")
	t.Setenv("APP_FLAGS_VALUES", `{"users-list":false}`)
	t.Setenv(
		"APP_AUTH_API_KEYS",
		`[{"id":"test","sha256":"`+strings.Repeat("a", 64)+`","subject":"test","scopes":["users:read","users:write"]}]`,
	)
	c, err := config.Load(dir(t), "local")
	if err != nil {
		t.Fatal(err)
	}
	if c.Logging.Level != "warn" || c.Flags.Values["users-list"] != false || len(c.Auth.APIKeys[0].Scopes) != 2 {
		t.Fatal("overrides not applied")
	}
}

func TestValidationAndRedaction(t *testing.T) {
	// The test profile retains the public key used to verify the deployment guard.
	c, err := config.Load(dir(t), "test")
	if err != nil {
		t.Fatal(err)
	}
	original := c
	c.Logging.Level = "error"
	if !original.SameStatic(c) {
		t.Fatal("level should reload")
	}
	c.HTTP.Address = ":9090"
	if original.SameStatic(c) {
		t.Fatal("listener change allowed")
	}
	c = original
	c.Service.Environment = "production"
	if c.Validate() == nil {
		t.Fatal("development credential allowed in production")
	}
	c.Database.Driver = "postgres"
	c.Database.DSN = "postgres://sensitive"
	for _, value := range c.Redacted() {
		if text, ok := value.(string); ok && strings.Contains(text, "sensitive") {
			t.Fatal("secret leaked")
		}
	}
}

func TestUnknownKeys(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "base.yaml"), []byte("typo: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(d, "local"); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestProductionConfiguration(t *testing.T) {
	digest := sha256.Sum256([]byte("test-random-credential"))
	t.Setenv(
		"APP_AUTH_API_KEYS",
		`[{"id":"test","sha256":"`+hex.EncodeToString(digest[:])+`","subject":"test","scopes":["users:read"]}]`,
	)
	t.Setenv("APP_DATABASE_DRIVER", "postgres")
	t.Setenv("APP_DATABASE_DSN", "postgres://app:secret@localhost/app?sslmode=require")
	if _, err := config.Load(dir(t), "production"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidRuntimeValue(t *testing.T) {
	t.Setenv("APP_HTTP_READ_TIMEOUT", "not-a-duration-secret")
	_, err := config.Load(dir(t), "local")
	if err == nil || strings.Contains(err.Error(), "not-a-duration-secret") {
		t.Fatal("bad environment value not safely rejected")
	}
}

func TestDeploymentIgnoresFilesAndKeepsSelector(t *testing.T) {
	t.Setenv(
		"APP_AUTH_API_KEYS",
		`[{"id":"deploy","sha256":"`+strings.Repeat("a", 64)+`","subject":"deploy","scopes":["users:read"]}]`,
	)
	t.Setenv("APP_DATABASE_DRIVER", "postgres")
	t.Setenv("APP_DATABASE_DSN", "postgres://localhost/app")
	t.Setenv("APP_SERVICE_ENVIRONMENT", "local")
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "base.yaml"), []byte("unknown: [invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, environment := range []string{"production", "staging", "preview-123"} {
		for _, directory := range []string{d, filepath.Join(d, "missing")} {
			c, err := config.Load(directory, environment)
			if err != nil {
				t.Fatal(err)
			}
			if c.Service.Environment != environment || c.HTTP.Docs || c.HTTP.Address != ":8080" {
				t.Fatalf("unexpected deployment defaults: %+v", c.Redacted())
			}
		}
	}
}

func TestOptionalLocalFilesAndMapReplacement(t *testing.T) {
	t.Setenv(
		"APP_AUTH_API_KEYS",
		`[{"id":"test","sha256":"`+strings.Repeat("a", 64)+`","subject":"test","scopes":["users:read"]}]`,
	)
	t.Setenv("APP_FLAGS_VALUES", `{"another-flag":true}`)
	c, err := config.Load(t.TempDir(), "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Flags.Values) != 1 || c.Flags.Values["another-flag"] != true {
		t.Fatal("environment map must replace defaults")
	}
}

func TestShutdownAndMigrationBudgetsValidated(t *testing.T) {
	for _, key := range []string{
		"APP_SHUTDOWN_DRAIN_TIMEOUT",
		"APP_SHUTDOWN_CLEANUP_TIMEOUT",
		"APP_SHUTDOWN_TELEMETRY_TIMEOUT",
		"APP_DATABASE_MIGRATION_TIMEOUT",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "0s")
			if _, err := config.Load(dir(t), "test"); err == nil {
				t.Fatal("zero budget accepted")
			}
		})
	}
}

func TestLoggingFormatsAndColor(t *testing.T) {
	for _, format := range []string{"json", "text", "console"} {
		for _, color := range []string{"auto", "never"} {
			t.Setenv("APP_LOGGING_FORMAT", format)
			t.Setenv("APP_LOGGING_COLOR", color)
			c, err := config.Load(dir(t), "test")
			if err != nil || c.Logging.Format != format || c.Logging.Color != color {
				t.Fatalf("logging config: %s/%s: %v", format, color, err)
			}
			next := c
			if color == "auto" {
				next.Logging.Color = "never"
			} else {
				next.Logging.Color = "auto"
			}
			if c.SameStatic(next) {
				t.Fatal("color selection must require restart")
			}
		}
	}
	t.Setenv("APP_LOGGING_COLOR", "always")
	if _, err := config.Load(dir(t), "test"); err == nil {
		t.Fatal("invalid color policy accepted")
	}
	t.Setenv("APP_LOGGING_COLOR", "auto")
	t.Setenv("APP_LOGGING_FORMAT", "unknown")
	if _, err := config.Load(dir(t), "test"); err == nil {
		t.Fatal("invalid log format accepted")
	}
}

func TestFlagConfiguration(t *testing.T) {
	t.Setenv("APP_FLAGS_PROVIDER", "file")
	t.Setenv("APP_FLAGS_FILES", `["configs/flags.local.json","another.json"]`)
	t.Setenv("APP_FLAGS_VALUES", `{"enabled":true,"mode":"summary","regions":["us"],"settings":{"limit":10}}`)
	c, err := config.Load(dir(t), "test")
	if err != nil {
		t.Fatal(err)
	}
	if c.Flags.Provider != "file" || len(c.Flags.Files) != 2 || c.Flags.Values["mode"] != "summary" {
		t.Fatal("structured flag environment settings not loaded")
	}
	changed := c
	changed.Flags.Values = map[string]any{"enabled": false}
	if !c.SameStatic(changed) {
		t.Fatal("flag value updates should be dynamic")
	}
	changed.Flags.Files = []string{"different.json"}
	if c.SameStatic(changed) {
		t.Fatal("file source changes require restart")
	}
	for _, flags := range []config.Flags{
		{Provider: "file"},
		{Provider: "file", Files: []string{" "}},
		{Provider: "local", Values: map[string]any{"invalid": math.NaN()}},
		{Provider: "unsupported"},
	} {
		candidate := c
		flags.InitTimeout = c.Flags.InitTimeout
		candidate.Flags = flags
		if candidate.Validate() == nil {
			t.Fatal("invalid flag configuration accepted")
		}
	}
}

func TestStructuredFlagYAML(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("APP_AUTH_API_KEYS",
		`[{"id":"test","sha256":"`+strings.Repeat("a", 64)+`","subject":"test","scopes":["users:read"]}]`)
	source := []byte(`flags:
  values:
    mode: summary
    regions: [us, eu]
    settings:
      limit: 10
`)
	if err := os.WriteFile(filepath.Join(directory, "base.yaml"), source, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(directory, "test")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(c.Flags.Values["settings"])
	if err != nil || string(encoded) != `{"limit":10}` || c.Flags.Values["mode"] != "summary" {
		t.Fatal("nested YAML flag values not retained")
	}
}
