package featureflags

import (
	"context"
	"testing"
	"time"

	"boilerplate/internal/platform/observability"

	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
)

func TestLaunchDarklyAdapter(t *testing.T) {
	data := ldtestdata.DataSource()
	data.Update(data.Flag("users-list").VariationForAll(true))
	client, err := ld.MakeCustomClient(
		"test-key",
		ld.Config{DataSource: data, Events: ldcomponents.NoEvents()},
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{client: client, logger: observability.Discard()}
	p.Update(nil)
	defer func() { _ = p.Close(context.Background()) }()
	if !p.Bool(context.Background(), "users-list", "test-user", false) {
		t.Fatal("provider value ignored")
	}
	data.Update(data.Flag("users-list").VariationForAll(false))
	if p.Bool(context.Background(), "users-list", "test-user", true) {
		t.Fatal("provider update ignored")
	}
	if !p.Bool(context.Background(), "missing", "test-user", true) {
		t.Fatal("provider fallback ignored")
	}
}
