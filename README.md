# Go microservice boilerplate

A Go service reference with feature-oriented clean architecture, constructor injection,
Gin routing with Huma code-first OpenAPI, validated environment configuration, and explicit
resource ownership.
No database server or SaaS account is required for local development.

## Quick start

Use Go **1.27.3** (CI/container reference version) and Make. The module minimum is Go 1.26.

```sh
go mod download
make hooks-install          # enable pre-commit lint, unit tests and OpenAPI generation
make dev                    # Air rebuilds and restarts on changes
# make run                  # single run without a watcher
```

In another terminal:

```sh
curl http://127.0.0.1:8080/livez
curl -X POST http://127.0.0.1:8080/api/v1/users \
  -H 'X-API-Key: local-development-key' \
  -H 'Content-Type: application/json' \
  -d '{"name":"Ada Lovelace","email":"ada@example.com"}'
curl -H 'X-API-Key: local-development-key' http://127.0.0.1:8080/api/v1/users
```

[Interactive reference](http://127.0.0.1:8080/docs) and
[OpenAPI JSON](http://127.0.0.1:8080/openapi.json) are enabled in local/test environments.
Memory persistence is non-durable: users disappear on restart. The public development
API key and users memory storage are prohibited outside local/test. Deployments read environment
variables without configuration files; optional YAML is a local/test convenience.

## Docker

```sh
make docker-up             # compiled service; memory persistence, no SaaS required
make docker-dev            # Air + source bind mount + dependency/build caches
make docker-down           # stop the stack; retained volumes are not deleted
```

Both expose `http://127.0.0.1:8080`. Do not run the host and container server on the same port;
set `SERVICE_PORT=8081` for a different Compose host port. See [development](docs/development.md)
for optional PostgreSQL/telemetry and [operations](docs/operations.md) for production images.

## Included

- Users domain/use cases, HTTP DTOs, memory and PostgreSQL repositories.
- API-key authentication and RS256 JWT verification using static public keys or JWKS.
- Structured `slog`, colored local console, component initialization timing, configurable levels, OTLP logs/metrics/traces, New Relic OTLP support.
- Local feature flags or LaunchDarkly, with explicit fallback values.
- SIGHUP reload for log level/local flag values; other changes require restart.
- SIGINT/SIGTERM draining, startup unwinding, bounded resource cleanup.
- Testify assertions and feature-local Mockery generation; unit, database integration, real-process e2e and architecture-boundary checks.
- Gin HTTP routing with Huma-generated OpenAPI 3.1 and a pinned development-tool workflow.

Email, cron, Redis, job queues, identity issuance and cloud deployment are deliberately
outside the baseline. Authentication and user records are separate concepts.

## Folder structure

The project separates business rules, application workflows and infrastructure:

```text
cmd/                         Executable entry points
├── service/                 Starts the service; handles signals and migration mode
├── openapi/                 Generates the OpenAPI specification
└── dependencies/            Checks the reviewed dependency inventory

internal/                    Private application code
├── app/                     Wires dependencies and owns startup/shutdown cleanup
├── config/                  Loads, validates and reloads configuration
├── buildinfo/               Version, commit and build metadata
├── architecture/            Tests that enforce dependency boundaries
├── platform/                Technical capabilities shared across features
│   ├── httpserver/          Gin/Huma server, middleware and health endpoints
│   ├── auth/                API-key and JWT verification
│   ├── database/            Database connections and pool configuration
│   ├── featureflags/        Local and LaunchDarkly flag evaluation
│   └── observability/       Structured logs, metrics and traces
└── users/                   Example business feature
    ├── domain/              User entity, validation and business rules
    ├── application/         Use cases, permissions and dependency interfaces
    └── adapters/            HTTP handlers and memory/PostgreSQL persistence

api/                         Generated OpenAPI specification
configs/                     Base and local/test YAML configuration
migrations/                  SQL migrations and migration runner
scripts/                     Build, release and development helpers
tests/                       Database integration and real-process e2e tests
deploy/                      Supporting configuration for the optional OTel collector
docs/                        Architecture, operations and engineering guidance
.github/                     CI workflows
```

At the root, `AGENTS.md` guides AI agents. `Makefile`, `.golangci.yml`, `.editorconfig`
and `.air.toml` define development commands, code quality, editor settings and hot reload.
The Dockerfile and Compose files provide container workflows. Unit tests live beside the code
in `*_test.go` files; integration and e2e suites live under `tests/`.

A typical request follows:

```text
HTTP adapter → application use case → domain rules / repository interface → persistence adapter
```

`internal/app` connects these pieces through constructor injection. Business code stays independent
of Gin, PostgreSQL and external providers. These boundaries describe responsibilities, not a requirement
to add a directory or interface for every small change. See [architecture](docs/architecture.md) for details.

## Commands

```sh
make hooks-install         # enable tracked pre-commit checks for this clone
make check                 # dependency review, format, vet, lint, tests, contract drift
make mocks                 # regenerate selected dependency mocks
make mocks-check           # check generated mock drift without modifying files
make test-race
make test-e2e              # built executable + real HTTP + signals
make openapi               # regenerate api/openapi.json
make build                 # embeds build identity; inspect bin/service --version
make dev-postgres          # optional PostgreSQL 17 + migration + host Air
make docker-config         # validate both Compose configurations
make deps-check            # fail on unreviewed module graph/version changes
make vuln                  # reachable vulnerability analysis
```

The first dev/lint/vulnerability run downloads the pinned tool. Task aliases are optional:
`make task ARGS=check` or an installed `task check`. Exact tool versions are in Makefile.

For PostgreSQL, telemetry, production credentials and tests, follow the guides below.

## Documentation

- [Architecture](docs/architecture.md) and [decisions](docs/decisions/0001-architecture.md)
- [Development and Air](docs/development.md)
- [Dependency policy and audit](docs/dependencies.md)
- [Configuration](docs/configuration.md)
- [API and OpenAPI](docs/api.md)
- [Authentication](docs/authentication.md)
- [Observability and New Relic](docs/observability.md)
- [Feature flags](docs/feature-flags.md)
- [Code quality policies and migration report](docs/code-quality.md)
- [Testing](docs/testing.md) and [initial verification](docs/verification.md)
- [Operations and migrations](docs/operations.md)
- [Releases and build identity](docs/releases.md)
- [Twelve-factor alignment](docs/twelve-factor.md)
- [Users reference feature](internal/users/README.md)
- [Contributing](CONTRIBUTING.md) and [AI instructions](AGENTS.md)

Generated artifacts are reproducible from Go code. Provider integrations can be tested
locally with controlled test doubles; live New Relic/LaunchDarkly accounts are optional.
