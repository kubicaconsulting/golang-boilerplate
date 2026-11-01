package featureflags_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/featureflags"
	"boilerplate/internal/platform/observability"

	"github.com/stretchr/testify/require"
)

func flagFile(t *testing.T, data string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "flags.json")
	require.NoError(t, os.WriteFile(file, []byte(data), 0600))
	return file
}

func TestFileFlags(t *testing.T) {
	file := flagFile(t, `{
  "flagValues": {
   "mode": "detailed", "regions": ["us", "eu"], "settings": {"limit": 25}, "wrong": false
  },
  "flags": {
   "targeted": {"key":"targeted", "on":true, "variations":[false,true],
    "targets":[{"variation":1,"values":["preview-user"]}], "fallthrough":{"variation":0}, "offVariation":0},
   "segmented": {"key":"segmented", "on":true, "variations":[false,true],
    "rules":[{"variation":1,"clauses":[{"attribute":"key","op":"segmentMatch","values":["preview"]}]}],
    "fallthrough":{"variation":0}, "offVariation":0}
  },
  "segments": {"preview":{"key":"preview","included":["preview-user"],"excluded":[],"rules":[]}}
 }`)
	provider, err := featureflags.NewProvider(config.Flags{
		Provider: "file", Files: []string{file}, InitTimeout: time.Second,
		Values: map[string]any{"mode": "summary"},
	}, observability.Discard())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Close(t.Context())) })

	require.True(t, provider.Bool(t.Context(), "targeted", "preview-user", false))
	require.False(t, provider.Bool(t.Context(), "targeted", "other", true))
	require.True(t, provider.Bool(t.Context(), "segmented", "preview-user", false))
	require.False(t, provider.Bool(t.Context(), "segmented", "other", true))
	mode, err := provider.Enum(t.Context(), "mode", "reader", "summary", "summary", "detailed")
	require.NoError(t, err)
	require.Equal(t, "detailed", mode)
	require.Equal(t, []string{"us", "eu"}, provider.StringList(t.Context(), "regions", "reader", nil))
	raw, err := provider.JSON(t.Context(), "settings", "reader", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"limit":25}`, string(raw))
	require.Equal(t, "summary", provider.String(t.Context(), "wrong", "reader", "summary"))
	require.Equal(t, "summary", provider.String(t.Context(), "missing", "reader", "summary"))
	require.Equal(t, "summary", provider.String(t.Context(), "wrong", "", "summary"))
	require.NoError(t, provider.Close(t.Context()))
}

func TestMultipleFlagFiles(t *testing.T) {
	first := flagFile(t, `{"flagValues":{"one":true}}`)
	second := flagFile(t, `{"flagValues":{"two":"ready"}}`)
	provider, err := featureflags.NewProvider(config.Flags{
		Provider: "file", Files: []string{first, second}, InitTimeout: time.Second,
	}, observability.Discard())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Close(t.Context())) })
	require.True(t, provider.Bool(t.Context(), "one", "reader", false))
	require.Equal(t, "ready", provider.String(t.Context(), "two", "reader", "fallback"))
}

func TestInvalidFlagFilesFailStartup(t *testing.T) {
	valid := flagFile(t, `{"flagValues":{"one":true}}`)
	for _, test := range []struct {
		name  string
		files []string
	}{
		{"empty", nil},
		{"missing", []string{filepath.Join(t.TempDir(), "missing.json")}},
		{"malformed", []string{flagFile(t, `{"flagValues":`)}},
		{"duplicate keys", []string{valid, valid}},
		{"valid and missing", []string{valid, filepath.Join(t.TempDir(), "missing.json")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider, err := featureflags.NewProvider(config.Flags{
				Provider: "file", Files: test.files, InitTimeout: time.Second,
			}, observability.Discard())
			require.Error(t, err)
			require.Nil(t, provider)
		})
	}
}
