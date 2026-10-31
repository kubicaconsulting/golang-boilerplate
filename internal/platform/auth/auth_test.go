package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"boilerplate/internal/config"
	"boilerplate/internal/platform/auth"

	"github.com/golang-jwt/jwt/v5"
)

func TestAPIKeys(t *testing.T) {
	sum := sha256.Sum256([]byte("secret"))
	a, err := auth.NewAuthenticator(
		context.Background(),
		config.Auth{
			APIKeys: []config.APIKey{
				{
					ID:      "key",
					SHA256:  hex.EncodeToString(sum[:]),
					Subject: "machine",
					Scopes:  []string{"users:read"},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		key, bearer string
		ok          bool
	}{{"secret", "", true}, {"wrong", "", false}, {"", "", false}, {"secret", "Bearer token", false}} {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		if input.key != "" {
			r.Header.Set("X-Api-Key", input.key)
		}
		if input.bearer != "" {
			r.Header.Set("Authorization", input.bearer)
		}
		_, err = a.Authenticate(r)
		if (err == nil) != input.ok {
			t.Fatalf("unexpected auth result %v", err)
		}
	}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r.Header.Add("X-Api-Key", "secret")
	r.Header.Add("X-Api-Key", "secret")
	if _, authErr := a.Authenticate(r); authErr == nil {
		t.Fatal("ambiguous credentials accepted")
	}
}

func TestJWT(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	a, err := auth.NewAuthenticator(
		context.Background(),
		config.Auth{
			JWT: config.JWT{
				Enabled:      true,
				Issuer:       "issuer",
				Audience:     "service",
				PublicKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		name, issuer, audience, algorithm string
		expiry                            time.Time
		ok                                bool
	}{
		{"valid", "issuer", "service", "RS256", time.Now().Add(time.Hour), true},
		{"expired", "issuer", "service", "RS256", time.Now().Add(-time.Minute), false},
		{"wrong issuer", "other", "service", "RS256", time.Now().Add(time.Hour), false},
		{"wrong audience", "issuer", "other", "RS256", time.Now().Add(time.Hour), false},
		{"missing expiry", "issuer", "service", "RS256", time.Time{}, false},
		{"wrong algorithm", "issuer", "service", "HS256", time.Now().Add(time.Hour), false},
	} {
		t.Run(input.name, func(t *testing.T) {
			claims := jwt.MapClaims{"sub": "user", "iss": input.issuer, "aud": input.audience, "scope": "users:read"}
			if !input.expiry.IsZero() {
				claims["exp"] = input.expiry.Unix()
			}
			method := jwt.GetSigningMethod(input.algorithm)
			var signing any = private
			if input.algorithm == "HS256" {
				signing = []byte("secret")
			}
			token, signingErr := jwt.NewWithClaims(method, claims).SignedString(signing)
			if signingErr != nil {
				t.Fatal(signingErr)
			}
			r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			p, authErr := a.Authenticate(r)
			if (authErr == nil) != input.ok {
				t.Fatal(authErr)
			}
			if input.ok && (p.Subject != "user" || len(p.Scopes) != 1) {
				t.Fatal(p)
			}
		})
	}
}

func TestJWKS(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []any{
				map[string]any{
					"kty": "RSA",
					"use": "sig",
					"alg": "RS256",
					"kid": "first",
					"n":   base64.RawURLEncoding.EncodeToString(private.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(private.E)).Bytes()),
				},
			},
		})
	}))
	defer receiver.Close()
	a, err := auth.NewAuthenticator(
		context.Background(),
		config.Auth{
			JWT: config.JWT{Enabled: true, Issuer: "issuer", Audience: "service", JWKSURL: receiver.URL},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close(context.Background()) }()
	token := jwt.NewWithClaims(
		jwt.SigningMethodRS256,
		jwt.MapClaims{
			"sub": "test",
			"iss": "issuer",
			"aud": "service",
			"exp": time.Now().Add(time.Hour).Unix(),
		},
	)
	token.Header["kid"] = "first"
	signed, err := token.SignedString(private)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	if _, err = a.Authenticate(r); err != nil {
		t.Fatal(err)
	}
}

func TestPrincipalScopesAreIsolated(t *testing.T) {
	sum := sha256.Sum256([]byte("secret"))
	a, err := auth.NewAuthenticator(
		context.Background(),
		config.Auth{
			APIKeys: []config.APIKey{
				{
					ID:      "key",
					SHA256:  hex.EncodeToString(sum[:]),
					Subject: "machine",
					Scopes:  []string{"users:read"},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r.Header.Set("X-Api-Key", "secret")
	first, err := a.Authenticate(r)
	if err != nil {
		t.Fatal(err)
	}
	first.Scopes[0] = "users:write"
	next, err := a.Authenticate(r)
	if err != nil || next.Scopes[0] != "users:read" {
		t.Fatal("principal permissions shared between requests")
	}
}

func TestJWTRejectsInvalidTimingAndSignature(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	a, err := auth.NewAuthenticator(
		context.Background(),
		config.Auth{
			JWT: config.JWT{
				Enabled:      true,
				Issuer:       "issuer",
				Audience:     "service",
				PublicKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing subject", "future iat", "future nbf", "wrong signature"} {
		t.Run(name, func(t *testing.T) {
			c := jwt.MapClaims{"sub": "user", "iss": "issuer", "aud": "service", "exp": time.Now().Add(time.Hour).Unix()}
			key := private
			switch name {
			case "missing subject":
				delete(c, "sub")
			case "future iat":
				c["iat"] = time.Now().Add(time.Minute).Unix()
			case "future nbf":
				c["nbf"] = time.Now().Add(time.Minute).Unix()
			case "wrong signature":
				key = other
			}
			signed, signingErr := jwt.NewWithClaims(jwt.SigningMethodRS256, c).SignedString(key)
			if signingErr != nil {
				t.Fatal(signingErr)
			}
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+signed)
			if _, signingErr = a.Authenticate(request); signingErr == nil {
				t.Fatal("invalid credential accepted")
			}
		})
	}
}

func TestJWKSCacheAndRotation(t *testing.T) {
	first, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var current atomic.Pointer[rsa.PrivateKey]
	current.Store(first)
	var requests atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		key := current.Load()
		kid := "first"
		if key == second {
			kid = "second"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []any{
				map[string]any{
					"kty": "RSA",
					"use": "sig",
					"alg": "RS256",
					"kid": kid,
					"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
				},
			},
		})
	}))
	defer receiver.Close()
	a, err := auth.NewAuthenticator(
		context.Background(),
		config.Auth{
			JWT: config.JWT{Enabled: true, Issuer: "issuer", Audience: "service", JWKSURL: receiver.URL},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close(context.Background()) }()
	authenticate := func(key *rsa.PrivateKey, kid string) error {
		token := jwt.NewWithClaims(
			jwt.SigningMethodRS256,
			jwt.MapClaims{
				"sub": "test",
				"iss": "issuer",
				"aud": "service",
				"exp": time.Now().Add(time.Hour).Unix(),
			},
		)
		token.Header["kid"] = kid
		signed, signingErr := token.SignedString(key)
		if signingErr != nil {
			return signingErr
		}
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		request.Header.Set("Authorization", "Bearer "+signed)
		_, signingErr = a.Authenticate(request)
		return signingErr
	}
	if err = authenticate(first, "first"); err != nil {
		t.Fatal(err)
	}
	if err = authenticate(first, "first"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatal("cached keys fetched again")
	}
	current.Store(second)
	deadline := time.Now().Add(3 * time.Second)
	for {
		err = authenticate(second, "second")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rotation failed", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if requests.Load() != 2 {
		t.Fatalf("unexpected key fetches: %d", requests.Load())
	}
}

func TestJWKSShutdownCancelsRefresh(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	canceled := make(chan struct{})
	receiver := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
	}))
	defer receiver.Close()
	a, err := auth.NewAuthenticator(
		context.Background(),
		config.Auth{
			JWT: config.JWT{Enabled: true, Issuer: "issuer", Audience: "service", JWKSURL: receiver.URL},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close(context.Background()) }()
	token := jwt.NewWithClaims(
		jwt.SigningMethodRS256,
		jwt.MapClaims{
			"sub": "test",
			"iss": "issuer",
			"aud": "service",
			"exp": time.Now().Add(time.Hour).Unix(),
		},
	)
	token.Header["kid"] = "key"
	signed, err := token.SignedString(private)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+signed)
	finished := make(chan error, 1)
	go func() {
		_, authErr := a.Authenticate(request)
		finished <- authErr
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("key fetch did not start")
	}
	if err = a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("key fetch outlived cleanup")
	}
	select {
	case err = <-finished:
		if err == nil {
			t.Fatal("unverified token accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("authentication remained blocked")
	}
}

// unexpectedTransport models a process whose default HTTP transport was replaced.
type unexpectedTransport struct{}

func (unexpectedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected transport must never be used")
}

//nolint:reassign // Simulate a foreign transport replacement in this nonparallel test; cleanup restores it.
func TestJWKSRejectsIncompatibleDefaultTransport(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = unexpectedTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	authenticator, err := auth.NewAuthenticator(t.Context(), config.Auth{JWT: config.JWT{
		Enabled: true, Issuer: "issuer", Audience: "service", JWKSURL: "https://identity.example.invalid/keys",
	}})
	if err == nil || authenticator != nil {
		t.Fatal("incompatible default transport must return an error rather than panic")
	}
}
