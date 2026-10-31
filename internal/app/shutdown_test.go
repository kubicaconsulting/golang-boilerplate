package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"boilerplate/internal/config"
)

func TestDrainTimeoutPreservesCleanupAndTelemetry(t *testing.T) {
	signals := make(chan string, 20)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		signals <- r.URL.Path
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", receiver.URL)
	c, err := config.Load("../../configs", "test")
	if err != nil {
		t.Fatal(err)
	}
	c.Telemetry.Enabled = true
	c.Shutdown.DrainTimeout = 30 * time.Millisecond
	c.Shutdown.CleanupTimeout = 300 * time.Millisecond
	c.Shutdown.TelemetryTimeout = 2 * time.Second
	runtime, err := NewRuntime(context.Background(), c, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close(context.Background()) }()
	cleanupStarted := make(chan error, 1)
	runtime.resources.Add("slow", func(ctx context.Context) error {
		cleanupStarted <- ctx.Err()
		<-ctx.Done()
		return ctx.Err()
	})
	entered := make(chan struct{})
	server := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(entered)
			<-r.Context().Done()
		}),
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runtime.serve(ctx, server, listener) }()
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		req, e := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+listener.Addr().String(), nil)
		if e != nil {
			return
		}
		res, e := client.Do(req)
		if e == nil {
			_ = res.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not enter")
	}
	cancel()
	select {
	case err = <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected timeout: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("shutdown exceeded budget")
	}
	if err = <-cleanupStarted; err != nil {
		t.Fatalf("cleanup inherited drain deadline: %v", err)
	}
	foundLogs := false
	for len(signals) > 0 {
		if <-signals == "/v1/logs" {
			foundLogs = true
		}
	}
	if !foundLogs {
		t.Fatal("final logs did not flush after cleanup deadline")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("forced close did not cancel request")
	}
}
