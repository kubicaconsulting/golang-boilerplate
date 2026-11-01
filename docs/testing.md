# Testing

```sh
make lint-config-check
make format-check
make lint
make test-unit
make mocks                 # regenerate selected interface mocks
make mocks-check           # verify generated output without changing source
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

## Assertions and generated mocks

Use standard Go testing with Testify `require` for prerequisites that must succeed and `assert`
for independent outcomes. Use `require.ErrorIs` for wrapped/sentinel errors and `require.NoError`
before inspecting successful results. Existing straightforward standard-library assertions remain
valid; a new framework does not require rewriting unrelated tests.

Mockery generates Testify mocks for interfaces explicitly listed in `.mockery.yml`. The starter
selects the users application's `Repository` and `Flags` ports. Output is a single `mocks_test.go`
file beside the interfaces, in the external `application_test` package. Generated constructors
register expectation checks through `t.Cleanup`; create a fresh mock for each test/subtest.

```go
repository := NewMockRepository(t)
flags := NewMockFlags(t)
actor := application.Actor{Subject: "reader", Scopes: []string{"users:read"}}
flags.EXPECT().Bool(t.Context(), "users-list", actor.Subject, true).Return(false).Once()
service := application.NewService(repository, flags)

_, err := service.List(t.Context(), actor, 20, 0)

require.ErrorIs(t, err, application.ErrDisabled)
```

This verifies the disabled-feature outcome and that persistence is not called: the repository
has no expected calls, so an unexpected access fails the test. Use exact arguments where they
express a contract, and matchers for generated IDs/timestamps or other incidental values. Avoid
locking tests to private implementation details or unnecessary call ordering.

For another feature, add its consumer-owned package/interface names to `.mockery.yml`, retain
`filename: mocks_test.go` and an external test package, then run `make mocks`. Commit generated
files alongside interface changes; never edit them manually. Remove obsolete generated files
when removing a package from the configuration. Small helpers should serve real repeated setup;
the users tests demonstrate a feature-local service builder without a shared test framework.

`make check` includes `make mocks-check`, so CI rejects missing, stale or obsolete mock output.
The check generates in an isolated temporary copy of tracked and non-ignored new files, compares
results and leaves the working tree and index untouched. Git and tar are required for this check;
Go and the pinned generator may need initial dependency downloads. `make test-unit` and the commit
hook use the checked-in mocks, keeping generation out of the normal fast test loop.

Mocks cover application decisions and failures. Keep the in-memory adapter for local operation
and adapter tests, real PostgreSQL for persistence integration, and real processes for lifecycle
e2e. Testify and Mockery are test/development dependencies, not production DI infrastructure.

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
