# Operations

## Build and startup

`make build` produces bin/service. Dockerfile builds a non-root static image with configuration
files and embedded SQL. The image selects production; deployment must explicitly choose its
persistence mode and configure credentials. Deployments load environment variables without requiring configuration files; HTTP defaults to :8080.
The users feature rejects memory storage outside local/test.

Startup validates before allocating resources, then initializes logging/telemetry, persistence,
authentication, flags and HTTP. Partial failure unwinds resources. Startup never auto-migrates.
No email/cron/queue/Redis process starts implicitly.

## Docker and Compose

`make docker-up` builds/runs the compiled image with local defaults. `make docker-dev` selects
the Air development target with source/cache mounts. `make docker-config` validates both files.
The final Dockerfile target is production: non-root distroless, compiled binary/configs only,
without Air, Go or source mounts. It selects APP_ENV=production; configure authentication and
persistence explicitly. Compose defaults are for local use and override that environment.

Use `VERSION=<unique-id> make release-image` for a reviewed release image (see releases.md), and inject
secrets at runtime. `docker build --target production -t microservice .` remains useful for local verification. `docker compose ... run --build --rm api --migrate` uses the same image for explicit schema
upgrades. Configurable localhost port mappings and a 35-second stop grace cover the service's
20-second combined shutdown budget and Air's 25-second interrupt budget. See development.md for commands
that correctly use Compose hostnames and optional dependency profiles.

## PostgreSQL

```sh
docker compose --profile database up -d postgres
export APP_DATABASE_DRIVER=postgres
export APP_DATABASE_DSN='postgres://app:local-only-password@localhost:5432/app?sslmode=disable'
make migrate
make run
```

Use TLS/verification and separate runtime/migration database permissions in real deployments.
DSNs are parsed by pgx; credentials are not interpolated into a hand-built URL. All configured
pool limits/lifetimes are applied. Choose limits against aggregate service replica counts.

Migration mode is `go run ./cmd/service --env production --migrate` with the same environment
configuration. SQL files are embedded, lexically ordered, checksum-tracked and applied inside
one transaction with a transaction-scoped advisory lock. Each new file needs a zero-padded
sequence number. Applied files are immutable. Add a new forward migration; destructive rollback
is not automated. This runner supports transactional SQL only: concurrent-index/other
nontransactional migrations require an explicit tool/runner decision before adding them.
Use backward-compatible expand/contract changes during rolling deployments.

## Readiness and shutdown

/livez is dependency-independent. /readyz returns 503 before readiness, while draining, or if the
required database ping fails. Dependency errors are not exposed. Optional telemetry/LaunchDarkly
outages do not remove readiness. Configure deployment probes directly against these endpoints.

SIGINT/SIGTERM mark unready, close listeners and drain HTTP, then close providers/database and
flush telemetry. The default budgets are HTTP drain 10s, infrastructure cleanup 5s, then telemetry flush 5s.
Forced HTTP close follows drain timeout. Cleanup and flush receive fresh contexts; an expired drain
cannot consume either budget, and a slow resource cannot consume the reserved telemetry phase.
Resource callbacks run in reverse registration order, dividing remaining cleanup time among remaining
callbacks. Telemetry similarly divides its phase among metrics, traces and logs. Every callback is
attempted even after errors. Non-cooperative resource callbacks can outlive their deadline in a
goroutine; timeout/error is reported and the executable exits with failure. Provision termination
grace above the sum of all three budgets plus load-balancer propagation time. Changing these settings
requires corresponding changes to Compose stop_grace_period and Air kill_delay.
Migration execution has its own database.migration_timeout (30s), followed by fresh bounded cleanup.

SIGHUP reload rules are in configuration.md. Put TLS and deployment-specific request throttling
at your chosen gateway or add an explicit transport adapter policy. Forwarded client IP headers
are not used for authorization/rate limiting. No browser CORS policy is assumed.

Container builds, provider credentials, actual cloud rollout and live SaaS delivery require the
corresponding deployment environment; passing local tests is not a deployment certification.
