# Authentication and authorization

Protected users operations accept either X-API-Key or Authorization: Bearer. Credentials are
translated to a provider-independent principal, then an application Actor. Missing/invalid,
duplicate or mixed credentials fail with 401; there is no anonymous fallback. Health is public.
Application use cases independently require users:read/users:write and return 403 when absent.
These are service-level permissions; tenant ownership is not assumed by the reference feature.

## API keys

Use high-entropy randomly generated keys. Store only SHA-256 digests in APP_AUTH_API_KEYS:

```json
[{"id":"service-a-v1","sha256":"<64 hex characters>","subject":"service-a","scopes":["users:read"]}]
```

Verification compares digests in constant time. Rotation: configure old and new keys with
unique IDs/digests, restart, move callers, then remove the old key and restart. A production
secret-store key lookup can replace configured keys later. SHA-256 is suitable for random API
keys, not user passwords. Never use this mechanism as password storage.

The public `local-development-key` exists only in local/test YAML. Its digest is rejected in
staging/production. Keys, their headers and hashes are not included in request logs.

## JWT

Set APP_AUTH_JWT_ENABLED=true, APP_AUTH_JWT_ISSUER and APP_AUTH_JWT_AUDIENCE. Configure exactly
one of APP_AUTH_JWT_JWKS_URL (HTTPS) or APP_AUTH_JWT_PUBLIC_KEY_PEM (RSA public key).
Verification permits RS256 only, requires exp, issuer, audience and sub, and validates nbf/iat
when present with 15 seconds of clock leeway. The space-separated scope claim supplies permissions.
Tokens are never issued by this service. No identity-provider account is required for tests.

JWKS keys are fetched lazily by the established CoreOS remote key set and cached in memory.
Unknown key IDs request a refresh; concurrent fetches are coalesced, the HTTP adapter permits
at most one refresh per second, and requests have a five-second timeout/one-MiB body bound.
Lifetime cancellation closes detached refresh requests during shutdown. Existing cached keys
can verify during an upstream outage; unknown/unavailable keys fail closed. Static RSA PEM
keys must be at least 2048 bits; configure suitably strong keys at your remote issuer too.
Static PEM rotation requires restart. Provider URLs/key configuration are trusted deployment
configuration. Always use TLS at the deployment boundary; proxy identity headers are not trusted.

CoreOS verifies signatures with an explicit RS256 allowlist and validates issuer/audience.
golang-jwt validates required exp, nbf/iat and the same issuer/audience with 15 seconds of leeway.
CoreOS's separate expiry policy is disabled only because the claim validator owns this policy;
missing expiry or invalid timing still fails authentication. This is token verification, not
an OIDC login/session or token-issuance implementation.
