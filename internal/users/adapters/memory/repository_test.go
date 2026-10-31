package memory_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"boilerplate/internal/users/adapters/memory"
	users "boilerplate/internal/users/domain"
)

func TestConcurrentUniqueness(t *testing.T) {
	r := memory.NewRepository()
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := range 20 {
		wg.Go(func() {
			u, _ := users.NewUser(strconv.Itoa(i), "Ada", "same@example.com", time.Now())
			results <- r.Create(context.Background(), u)
		})
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, users.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("successful writes: %d", success)
	}
}
