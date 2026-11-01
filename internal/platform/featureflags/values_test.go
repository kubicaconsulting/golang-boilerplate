package featureflags_test

import (
	"encoding/json"
	"math"
	"sync"
	"testing"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/featureflags"
	"boilerplate/internal/platform/observability"

	"github.com/stretchr/testify/require"
)

func TestTypedLocalValues(t *testing.T) {
	settings := map[string]any{"limit": 10}
	regions := []string{"us", "eu"}
	values := map[string]any{"mode": "detailed", "regions": regions, "settings": settings, "wrong": 42}
	provider, err := featureflags.NewProvider(config.Flags{Provider: "local", Values: values}, observability.Discard())
	require.NoError(t, err)
	settings["limit"] = 99
	regions[0] = "changed"
	values["mode"] = "changed"

	require.Equal(t, "detailed", provider.String(t.Context(), "mode", "reader", "summary"))
	mode, err := provider.Enum(t.Context(), "mode", "reader", "summary", "summary", "detailed")
	require.NoError(t, err)
	require.Equal(t, "detailed", mode)
	mode, err = provider.Enum(t.Context(), "mode", "reader", "summary", "summary", "compact")
	require.NoError(t, err)
	require.Equal(t, "summary", mode)
	_, err = provider.Enum(t.Context(), "mode", "reader", "invalid", "summary", "detailed")
	require.Error(t, err)
	require.Equal(t, "fallback", provider.String(t.Context(), "wrong", "reader", "fallback"))
	require.True(t, provider.Bool(t.Context(), "wrong", "reader", true))

	actual := provider.StringList(t.Context(), "regions", "reader", nil)
	require.Equal(t, []string{"us", "eu"}, actual)
	actual[0] = "changed"
	require.Equal(t, []string{"us", "eu"}, provider.StringList(t.Context(), "regions", "reader", nil))
	raw, err := provider.JSON(t.Context(), "settings", "reader", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"limit":10}`, string(raw))
	raw[0] = '['
	raw, err = provider.JSON(t.Context(), "settings", "reader", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"limit":10}`, string(raw))
	_, err = provider.JSON(t.Context(), "settings", "reader", json.RawMessage(`invalid`))
	require.Error(t, err)

	fallback := []string{"safe"}
	returned := provider.StringList(t.Context(), "wrong", "reader", fallback)
	returned[0] = "changed"
	require.Equal(t, []string{"safe"}, fallback)
	require.NoError(t, provider.Update(map[string]any{"regions": []any{"us", nil}, "mode": nil}))
	require.Equal(t, []string{"safe"}, provider.StringList(t.Context(), "regions", "reader", fallback))
	require.Equal(t, "summary", provider.String(t.Context(), "mode", "reader", "summary"))
	require.NoError(t, provider.Update(map[string]any{"mode": "compact"}))
	require.Error(t, provider.Update(map[string]any{"mode": math.NaN()}))
	require.Equal(t, "compact", provider.String(t.Context(), "mode", "reader", "summary"))
}

func TestConcurrentFlagUpdates(t *testing.T) {
	provider, err := featureflags.NewProvider(config.Flags{Provider: "local"}, observability.Discard())
	require.NoError(t, err)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				if updateErr := provider.Update(map[string]any{"enabled": true, "regions": []string{"us"}}); updateErr != nil {
					t.Error(updateErr)
					return
				}
				provider.Bool(t.Context(), "enabled", "reader", false)
				list := provider.StringList(t.Context(), "regions", "reader", nil)
				if len(list) != 1 || list[0] != "us" {
					t.Error("incomplete snapshot")
				}
			}
		})
	}
	workers.Wait()
}
