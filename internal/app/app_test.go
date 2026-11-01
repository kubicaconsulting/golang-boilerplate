package app_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"boilerplate/internal/app"
	"boilerplate/internal/config"
)

func TestReload(t *testing.T) {
	c, err := config.Load("../../configs", "test")
	if err != nil {
		t.Fatal(err)
	}
	r, err := app.NewRuntime(context.Background(), c, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()
	next := c
	next.Flags.Values = map[string]any{"users-list": false}
	next.Logging.Level = "warn"
	if err = r.Reload(next); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("X-Api-Key", "local-development-key")
	w := httptest.NewRecorder()
	r.HTTP.Handler.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("flag reload: %d", w.Code)
	}
	next.HTTP.Address = ":9000"
	if r.Reload(next) == nil {
		t.Fatal("static reload accepted")
	}
}

func TestListenFailureCleansUp(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	c, err := config.Load("../../configs", "test")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP.Address = listener.Addr().String()
	r, err := app.NewRuntime(context.Background(), c, "test")
	if err != nil {
		t.Fatal(err)
	}
	if r.Run(context.Background()) == nil {
		t.Fatal("expected listen failure")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = r.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUsersRequiresDurablePersistenceForDeployments(t *testing.T) {
	c, err := config.Load("../../configs", "test")
	if err != nil {
		t.Fatal(err)
	}
	c.Auth.APIKeys[0].SHA256 = strings.Repeat("a", 64)
	for _, environment := range []string{"production", "preview-123"} {
		c.Service.Environment = environment
		if _, err = app.NewRuntime(context.Background(), c, "test"); err == nil ||
			!strings.Contains(err.Error(), "durable persistence") {
			t.Fatalf("missing storage safeguard: %v", err)
		}
	}
}
