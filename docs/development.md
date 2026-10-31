# Development: Air and Docker Compose

## Host development

`make dev` uses the upstream [Air](https://github.com/air-verse/air) project pinned to v1.67.4.
No global Air install is required. `make run` starts once without watching. Both use the same
configuration loader, local authentication and optional-provider defaults.

.air.toml watches Go source, YAML and go.mod/go.sum. Test files, build outputs, generated API,
documentation and deployment files are excluded. After a change, Air compiles a fresh binary,
sends an interrupt to the old process, allows up to 25 seconds for shutdown, then starts the
replacement. The application shutdown budget is 20 seconds by default. Increase both budgets
if your local shutdown setting grows. Invalid builds stop the old process instead of serving
stale code. Air restarts reset memory persistence; use PostgreSQL when state must survive.

Air does not load .env (env_files is explicitly empty), matching the application's environment
policy. Export APP_* values or use local/test YAML. Configuration changes under Air restart
the process; SIGHUP reload is still useful when running without Air. Restart Air after editing
its own configuration. `make clean` removes host build artifacts.

## Container development

```sh
make docker-config
make docker-dev
```

This merges compose.yaml and compose.dev.yaml, selects Dockerfile's dev target, mounts source,
and starts Air directly. Polling handles Docker Desktop filesystem events. Named volumes cache
Go dependencies/builds and isolate container binaries from host binaries. No database or SaaS
provider starts by default. `make docker-up` runs the compiled non-root production target using
local configuration. Stop with `make docker-down`; volumes are retained.

Compose is a local-development setup. Dockerfile defaults to production configuration when
used without Compose. Do not deploy the local public key/default database password to a real
environment. Port mappings bind localhost. `SERVICE_PORT=8081 make docker-dev` changes only the
host API port; APP_HTTP_ADDRESS remains :8080 inside the container.

Compose reads .env for variable interpolation; it is not committed. Unlike host Air, Compose
forwards the APP_*/OTEL_* variables listed in compose.yaml. When adding a new setting used in
containers, update that explicit allowlist. Shared secrets stay outside the image/build args.

## Optional PostgreSQL

For host Air with the same PostgreSQL major version used by CI:

```sh
make dev-postgres
# POSTGRES_PORT=15432 make dev-postgres  # alternate host port
```

This explicitly starts the optional PostgreSQL 17 profile, waits for health, applies migrations,
and starts host Air. APP_DATABASE_DSN can override the local default. Keep test data in a separate
disposable database. The ordinary make dev command keeps its zero-infrastructure memory default.
Use PostgreSQL for persistence behavior and before promoting changes; memory does not reproduce
constraints, transactions or connection failures. Docker is required only for this optional workflow.
Task also exposes dev:postgres. For container Air:

```sh
docker compose --profile database up -d --wait postgres
APP_DATABASE_DRIVER=postgres \
APP_DATABASE_DSN='postgres://app:local-only-password@postgres:5432/app?sslmode=disable' \
  docker compose run --build --rm api --migrate
APP_DATABASE_DRIVER=postgres \
APP_DATABASE_DSN='postgres://app:local-only-password@postgres:5432/app?sslmode=disable' \
  make docker-dev
```

Migrations remain explicit. Wait for PostgreSQL before running migrations or the API. Inside
Compose, connect to hostname postgres; host processes use localhost and POSTGRES_PORT (5432
by default). PostgreSQL data uses a named volume. Deleting that volume deletes local data.

## Optional telemetry

```sh
docker compose --profile telemetry up -d collector
APP_TELEMETRY_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318 make docker-dev
```

Host-run processes use http://localhost:4318 instead. Optional profiles can be combined.
`docker compose logs -f api` shows the service; use the same -f file options for a dev stack.
Production and dev targets use different image tags, so switching modes cannot accidentally
reuse Air for a compiled migration command. SERVICE_IMAGE overrides the image repository name.
Dev dependencies are tooling only; Air is absent from the production image and service imports.
