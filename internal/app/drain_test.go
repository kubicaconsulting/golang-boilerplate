package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"boilerplate/internal/config"
)

func TestShutdownDrainsBeforeResources(t *testing.T) {
	c, err := config.Load("../../configs", "test")
	if err != nil {
		t.Fatal(err)
	}
	c.Shutdown.DrainTimeout = 2 * time.Second
	r, err := NewRuntime(context.Background(), c, "test")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	r.resources.Add("sentinel", func(context.Context) error {
		close(closed)
		return nil
	})
	r.HTTP.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	server := &http.Server{Handler: r.HTTP.Handler, ReadHeaderTimeout: time.Second}
	go func() { result <- r.serve(ctx, server, listener) }()
	requestDone := make(chan error, 1)
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	go func() {
		req, requestErr := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"http://"+listener.Addr().String(),
			nil,
		)
		if requestErr != nil {
			requestDone <- requestErr
			return
		}
		response, requestErr := client.Do(req)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				requestErr = errors.New("unexpected status")
			}
		}
		requestDone <- requestErr
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request never started")
	}
	cancel()
	select {
	case <-closed:
		t.Fatal("resource closed while request active")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err = <-requestDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	select {
	case <-closed:
	default:
		t.Fatal("resource not closed")
	}
}
