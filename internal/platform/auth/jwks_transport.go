package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const maxJWKSResponseBytes = 1 << 20

// jwksTransport bounds shared key refreshes and ties detached provider requests
// to the authenticator lifetime. Signature/key parsing stays in CoreOS/go-jose.
type jwksTransport struct {
	lifetime context.Context
	next     http.RoundTripper
	limit    *rate.Limiter
}

func newJWKSTransport(ctx context.Context, next http.RoundTripper) *jwksTransport {
	return &jwksTransport{ctx, next, rate.NewLimiter(rate.Every(time.Second), 1)}
}

func (t *jwksTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := t.lifetime.Err(); err != nil {
		return nil, err
	}
	if !t.limit.Allow() {
		return nil, errors.New("JWKS refresh rate limited")
	}

	// Tie a shared provider fetch to both request cancellation and authenticator shutdown.
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.lifetime, cancel)
	cleanup := func() {
		stop()
		cancel()
	}

	response, err := t.next.RoundTrip(r.WithContext(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	response.Body = &boundedBody{
		Reader:  io.LimitReader(response.Body, maxJWKSResponseBytes),
		body:    response.Body,
		cleanup: cleanup,
	}
	return response, nil
}

type boundedBody struct {
	io.Reader
	body    io.Closer
	cleanup func()
	once    sync.Once
}

func (b *boundedBody) Close() error {
	err := b.body.Close()
	b.once.Do(b.cleanup)
	return err
}
