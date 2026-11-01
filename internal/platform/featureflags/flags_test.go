package featureflags_test

import (
	"context"
	"testing"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/featureflags"
	"boilerplate/internal/platform/observability"
)

func TestLocalFlags(t *testing.T) {
	values := map[string]any{"feature": true}
	p, err := featureflags.NewProvider(config.Flags{Provider: "local", Values: values}, observability.Discard())
	if err != nil {
		t.Fatal(err)
	}
	values["feature"] = false
	if !p.Bool(context.Background(), "feature", "user", false) {
		t.Fatal("mutable input retained")
	}
	if err = p.Update(map[string]any{"feature": false}); err != nil {
		t.Fatal(err)
	}
	if p.Bool(context.Background(), "feature", "user", true) {
		t.Fatal("reload failed")
	}
	if !p.Bool(context.Background(), "unknown", "user", true) {
		t.Fatal("fallback ignored")
	}
}
