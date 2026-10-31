# ADR 0002: Deployment configuration, lifecycle phases and release identity

Status: accepted.

Deployed services must not depend on named environment files, silently use non-durable reference
storage, or lose cleanup/telemetry because HTTP drain exhausted a shared timeout.

Local/test retain optional YAML. Other labels use defaults and environment variables, with required
explicit persistence/authentication. The users composition root rejects memory outside local/test;
the database abstraction does not impose persistence on unrelated stateless services.

Shutdown has separate bounded HTTP, infrastructure and telemetry phases. Callbacks share the
remaining time within their phase, preserving attempts for later resources. Uncooperative callbacks
cannot be guaranteed to finish; errors are propagated. Migration gets a separate operation budget.

Build identity is embedded in binaries and OCI labels. Release construction requires clean committed
source and unique tags. CI archives the image without assuming a registry or deployment platform.
Registry immutability, artifact retention and same-digest promotion remain operator responsibilities.
