# Testing

```sh
make lint-config-check
make format-check
make lint
make test-unit
make test-race
make test-e2e
make check
make vuln
make deps-check
make docker-config
```

Unit/default tests cover domain normalization, use-case authorization/flags/cancellation,
configuration precedence/validation/redaction, auth strategies, local flag snapshots, concurrent
memory uniqueness, HTTP request isolation/validation/contracts, resource cleanup, reload policy
and a local OTLP receiver. Some default tests bind localhost, without external dependencies.

E2E builds the actual executable, opens a real listener, verifies users/auth/health, sends SIGHUP,
then SIGTERM and checks successful exit. Tests are controlled by build tags and run explicitly.
No Clerk, LaunchDarkly, New Relic or other SaaS credentials are needed.

## PostgreSQL integration

Use a dedicated disposable database. Integration tests apply migrations and write/delete users;
never point them at a shared or production database.

```sh
docker compose --profile database up -d postgres
# Create a separate database (or use a separate container) for tests:
docker compose exec postgres createdb -U app app_test
export TEST_DATABASE_DSN='postgres://app:local-only-password@localhost:5432/app_test?sslmode=disable'
make test-integration
E2E_DATABASE_DSN="$TEST_DATABASE_DSN" make test-e2e
```

Integration checks apply/checksum migrations, verify idempotence, real unique constraints,
repository round trips, cancellation, transaction rollback and configured pool limits. Missing
TEST_DATABASE_DSN is a failure, not a silent skip. E2E_DATABASE_DSN switches the full-process
suite to PostgreSQL and applies migrations before startup. It creates unique records and expects
a disposable database; discard it after the suite. CI provisions PostgreSQL and runs both memory
and PostgreSQL e2e modes.

Use narrow fakes when testing application decisions, real database semantics for SQL, and real
processes for signal behavior. Do not require 100% coverage or add tests that merely restate code.
The architecture test enforces imports; generated-contract comparison catches artifact drift.

Container changes additionally build Dockerfile's production and dev targets and smoke-test
both Compose modes. Verify Air by creating a user in memory mode, changing a watched source
file, and confirming the process restarts (the in-memory user disappears) without a bind error.
Use a temporary source copy to avoid changing real application files during this check.

The CI compiled-container smoke test generates a random API key for each run, injects only its
SHA-256 digest through `APP_AUTH_API_KEYS`, and sends the raw key with the protected request.
This overrides local YAML credentials so customizing `configs/local.yaml` cannot break CI
authentication. The key has only the `users:read` scope and is discarded with the test container.
