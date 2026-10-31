package app_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"boilerplate/internal/app"
)

func TestCleanupOrderAndErrors(t *testing.T) {
	var r app.Resources
	var order []int
	r.Add("database", func(context.Context) error {
		order = append(order, 1)
		return nil
	})
	r.Add("adapter", func(context.Context) error {
		order = append(order, 2)
		return errors.New("failed")
	})
	if r.Close(context.Background()) == nil {
		t.Fatal("missing error")
	}
	_ = r.Close(context.Background())
	if !reflect.DeepEqual(order, []int{2, 1}) {
		t.Fatal(order)
	}
}

func TestCleanupDeadline(t *testing.T) {
	var r app.Resources
	blocked := make(chan struct{})
	r.Add("blocked", func(context.Context) error {
		<-blocked
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	defer close(blocked)
	if !errors.Is(r.Close(ctx), context.DeadlineExceeded) {
		t.Fatal("deadline ignored")
	}
}

func TestCleanupReservesTimeForLaterResources(t *testing.T) {
	var r app.Resources
	observed := make(chan error, 1)
	r.Add("remaining", func(ctx context.Context) error {
		observed <- ctx.Err()
		return nil
	})
	r.Add("slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if !errors.Is(r.Close(ctx), context.DeadlineExceeded) {
		t.Fatal("missing timeout")
	}
	select {
	case err := <-observed:
		if err != nil {
			t.Fatalf("later cleanup inherited expired budget: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("later cleanup not attempted")
	}
}
