// Package auth authenticates credentials and exposes a provider-independent principal.
package auth

import (
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"boilerplate/internal/config"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	jwksRequestTimeout = 5 * time.Second
	jwtClockSkew       = 15 * time.Second
)

// ErrUnauthorized indicates missing, ambiguous or invalid credentials.
var ErrUnauthorized = errors.New("invalid credentials")

// Principal contains verified identity and scopes independent of the provider.
type Principal struct {
	Subject string
	Kind    string
	Issuer  string
	Scopes  []string
}

type principalKey struct{}

// WithPrincipal attaches a verified principal to the request context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the principal attached by the authentication adapter.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

type key struct {
	digest    []byte
	principal Principal
}

// Authenticator owns configured credential verifiers and optional JWKS resources.
type Authenticator struct {
	keys      []key
	jwt       config.JWT
	verifier  *oidc.IDTokenVerifier
	validator *jwt.Validator
	cancel    context.CancelFunc
	transport *http.Transport
}

type claims struct {
	jwt.RegisteredClaims
	Scope string `json:"scope"`
}

// NewAuthenticator constructs verifiers from validated authentication settings.
func NewAuthenticator(ctx context.Context, c config.Auth) (*Authenticator, error) {
	a := &Authenticator{jwt: c.JWT}

	for _, k := range c.APIKeys {
		digest, err := hex.DecodeString(k.SHA256)
		if err != nil || len(digest) != 32 {
			return nil, errors.New("invalid API key digest")
		}
		a.keys = append(a.keys, key{
			digest: digest,
			principal: Principal{
				Subject: k.Subject,
				Kind:    "api_key",
				Issuer:  "configuration",
				Scopes:  append([]string(nil), k.Scopes...),
			},
		})
	}

	if c.JWT.Enabled {
		if err := a.configureJWT(ctx, c.JWT); err != nil {
			return nil, err
		}
	}

	return a, nil
}

// Authenticate verifies exactly one credential and returns its trusted principal.
func (a *Authenticator) Authenticate(r *http.Request) (Principal, error) {
	// Reject ambiguous credentials instead of silently choosing an authentication strategy.
	keys := r.Header.Values("X-Api-Key")
	bearers := r.Header.Values("Authorization")
	if len(keys) > 1 || len(bearers) > 1 || (len(keys) > 0 && len(bearers) > 0) {
		return Principal{}, ErrUnauthorized
	}

	if len(keys) == 1 {
		digest := sha256.Sum256([]byte(keys[0]))
		for _, k := range a.keys {
			if subtle.ConstantTimeCompare(digest[:], k.digest) == 1 {
				principal := k.principal
				principal.Scopes = append([]string(nil), k.principal.Scopes...)
				return principal, nil
			}
		}
		return Principal{}, ErrUnauthorized
	}

	if len(bearers) != 1 || !a.jwt.Enabled {
		return Principal{}, ErrUnauthorized
	}
	parts := strings.Fields(bearers[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return Principal{}, ErrUnauthorized
	}

	verified, err := a.verifier.Verify(r.Context(), parts[1])
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	c := &claims{}
	if err = verified.Claims(c); err != nil || a.validator.Validate(c) != nil || c.Subject == "" {
		return Principal{}, ErrUnauthorized
	}

	return Principal{Subject: c.Subject, Kind: "jwt", Issuer: c.Issuer, Scopes: strings.Fields(c.Scope)}, nil
}

// Close cancels JWKS work and releases idle transport connections.
func (a *Authenticator) Close(context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.transport != nil {
		a.transport.CloseIdleConnections()
	}
	return nil
}

func (a *Authenticator) configureJWT(ctx context.Context, c config.JWT) error {
	var keys oidc.KeySet
	if c.PublicKeyPEM != "" {
		public, err := jwt.ParseRSAPublicKeyFromPEM([]byte(c.PublicKeyPEM))
		if err != nil || public.N.BitLen() < 2048 {
			return errors.New("JWT requires an RSA public key of at least 2048 bits")
		}
		keys = &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{public}}
	} else {
		lifetime, cancel := context.WithCancel(ctx)
		a.cancel = cancel
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			cancel()
			return errors.New("default HTTP transport must support isolated JWKS connections")
		}
		transport := base.Clone()
		a.transport = transport
		client := &http.Client{
			Transport: newJWKSTransport(lifetime, otelhttp.NewTransport(transport)),
			Timeout:   jwksRequestTimeout,
		}
		keys = oidc.NewRemoteKeySet(oidc.ClientContext(lifetime, client), c.JWKSURL)
	}
	// CoreOS verifies RS256 signatures, issuer and audience. The JWT validator below
	// is the single owner of exp/nbf/iat policy, including required exp and 15s leeway.
	a.verifier = oidc.NewVerifier(
		c.Issuer,
		keys,
		&oidc.Config{
			ClientID:             c.Audience,
			SupportedSigningAlgs: []string{oidc.RS256},
			SkipExpiryCheck:      true,
		},
	)
	a.validator = jwt.NewValidator(
		jwt.WithIssuer(c.Issuer),
		jwt.WithAudience(c.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(jwtClockSkew),
	)
	return nil
}
