# Architecture

The service is a feature-oriented layered application. A feature owns its domain rules,
use cases and adapters. Infrastructure stays replaceable at the application boundary.

```text
HTTP adapter -> application use cases -> domain
                        |
                 repository / flags ports
                        ^
            PostgreSQL / memory / flags adapters

internal/app constructs and owns the concrete graph.
```

Use these boundaries with the smallest useful structure. They describe responsibilities and dependency
direction, not a requirement to create a directory, interface and file for every layer of every feature.
Add only the domain rules, use cases and adapters needed today; keep related small declarations together.
Prefer domain names, direct constructor wiring and concrete implementations inside each boundary. New
abstractions must solve a current problem. See [code clarity and review](code-quality.md#human-readability-and-idiomatic-go).


- `cmd/service`: process exit, signals, configuration selection, explicit migration mode.
- `internal/app`: manual constructor injection, provider selection, resource cleanup.
- `internal/config`: typed snapshots, precedence and validation.
- `internal/users/domain`: user entity, normalization and invariant errors.
- `internal/users/application`: authorization, use cases, consumer-owned repository/flag ports.
- `internal/users/adapters`: HTTP translation and concrete persistence.
- `internal/platform`: Gin transport/auth/telemetry/provider plumbing; no users business rules.
- `migrations`: embedded SQL with checksum tracking and transactional application.

A database pool is shared, not recreated per feature. Each feature repository receives that
pool. Each HTTP adapter registers feature routes on the shared server. Domain models are
not HTTP DTOs. Application Actor is independent of an authentication provider's principal.

No universal repository or generic CRUD base class is used. The current single-insert use
case is already atomic. For a future multi-write use case, introduce a transaction-scoped
port owned by that application; all participating repositories must use the same transaction.
Do not open independent transactions in each repository method or wrap external side effects
in a database transaction. Add an outbox when atomic persistence and event delivery is required.

Memory persistence is a local/test adapter, not durable storage. PostgreSQL is a reference
adapter; it is optional for local development. The current users feature requires durable storage
outside local/test, so deployments need PostgreSQL until another durable adapter is implemented.
A future stateless service can remove this feature-specific requirement. Additional databases need
their own configuration and repository implementation, selected in the composition root.

A boundary test rejects external/domain dependencies, infrastructure imports in users use
cases, and lower-layer imports of internal/app. The narrow exception is the SDK-free
internal/platform/featureflags/definitions package: it contains shared flag key/fallback constants,
and the boundary test rejects non-standard imports there. This does not permit application imports
of the featureflags provider or LaunchDarkly SDK. Tests may use concrete adapters to assemble
fixtures. Extend enforcement when adding features.

## Foundation and extension policy

The baseline provides clear dependency boundaries, constructor injection, validated environment
configuration, authentication and use-case authorization, bounded resource cleanup, structured
observability, code-first API contracts and meaningful automated checks. These guarantees should
remain easy to understand and verify as features grow. The users feature is a working reference for
applying them; its database requirement belongs to that feature.

Extend patterns in response to a concrete requirement or failure mode:

| Pattern | Introduce when |
|---|---|
| Consumer-owned interface | A use case needs to isolate an external dependency or support interchangeable implementations; one implementation can be sufficient at a real boundary. |
| Strategy or factory | Actual business policies or implementation choices vary, and separating them makes the existing workflow clearer. |
| Transaction boundary | Multiple writes must succeed or fail together. |
| Transactional outbox | A durable state change and subsequent event delivery must remain consistent across failures. |
| Retry policy | A specific outbound operation has recoverable transient failures and safe repeat semantics; bound attempts and respect cancellation/deadlines. |
| Background processing | A real workload needs asynchronous execution, with defined delivery, failure recovery and shutdown behavior. |
| Richer domain model | Business invariants require controlled state transitions, value objects or aggregate consistency. |
| Shared abstraction | Existing consumers have the same responsibility and semantics, and sharing it removes meaningful duplication. |

Keep each extension in the feature or adapter that requires it. Start with direct code, concrete
implementations and cohesive files. Add a short design decision when a change crosses feature
boundaries or introduces a new operational dependency. Extend boundary checks and behavior tests
alongside the feature; the current application-import rule is specific to the users reference.

A proposed pattern should explain the problem it solves today, why the simpler approach is
insufficient, and how its benefit is verified. Patterns observed in another service are examples to
learn from, not requirements for this boilerplate.

## HTTP framework boundary

Gin v1.12.0 routes requests; Huma v2 uses its official humagin adapter for typed handlers, validation
and OpenAPI. Gin types stay in HTTP adapters and process bootstrap, never domain/application ports.
The engine is created with gin.New(), not gin.Default(): existing net/http middleware owns structured
request logging, panic recovery, security headers and OpenTelemetry. The same http.Server still owns
listeners, timeouts and graceful shutdown; do not call Gin Run/RunTLS/RunQUIC.

Process entry points set Gin release mode before registering routes, keeping debug registration output
out of structured logs and generated JSON. Application log levels remain controlled by configuration.
Server construction does not mutate Gin's global mode or install global writers. Embedded callers
should choose framework diagnostic mode once at their own bootstrap boundary.

Proxy trust is disabled, escaped path parameters are supported, and implicit trailing-slash redirects
are disabled. GET routes receive HEAD aliases after registration; Gin's method-not-allowed handling
retains 405/Allow behavior. Router fallbacks use net/http error responses. Huma operation declarations
and generated OpenAPI remain the public API source of truth.
