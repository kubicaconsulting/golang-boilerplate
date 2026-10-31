package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

type resource struct {
	name    string
	cleanup func(context.Context) error
}

// Resources closes owned resources in reverse construction order, once.
// Closers must honor context; the supervisor also bounds non-cooperative SDKs.
type Resources struct {
	mu    sync.Mutex
	items []resource
	once  sync.Once
	err   error
}

// Add registers a cleanup before shutdown begins.
func (r *Resources) Add(name string, cleanup func(context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items = append(r.items, resource{name: name, cleanup: cleanup})
}

// Close attempts registered cleanups once in reverse order within the caller deadline.
func (r *Resources) Close(ctx context.Context) error {
	r.once.Do(func() {
		// Release the registration lock before calling provider-owned cleanup code.
		r.mu.Lock()
		items := append([]resource(nil), r.items...)
		r.mu.Unlock()

		// Dependencies close after their consumers, in reverse registration order.
		for i, item := range slices.Backward(items) {
			var closeCtx context.Context
			var cancel context.CancelFunc
			if deadline, ok := ctx.Deadline(); ok {
				// Backward iteration retains the original index: i+1 resources remain.
				remainingResources := time.Duration(i + 1)
				budget := max(time.Until(deadline)/remainingResources, 0)
				closeCtx, cancel = context.WithTimeout(ctx, budget)
			} else {
				closeCtx, cancel = context.WithCancel(ctx)
			}

			// Bound waiting even if a third-party closer does not honor its context.
			done := make(chan error, 1)
			go func() { done <- item.cleanup(closeCtx) }()

			select {
			case err := <-done:
				if err != nil {
					r.err = errors.Join(r.err, fmt.Errorf("close %s: %w", item.name, err))
				}
			case <-closeCtx.Done():
				r.err = errors.Join(r.err, fmt.Errorf("close %s: %w", item.name, closeCtx.Err()))
			}

			cancel()
		}
	})

	return r.err
}
