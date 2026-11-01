# Testing

```sh
make lint-config-check
make format-check
make lint
make test-unit
make mocks                 # regenerate selected interface mocks
make mocks-check           # verify generated output without changing source
make test-race
make test-coverage          # race detection, reports and coverage threshold
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

## Coverage policy

CI runs `make test-coverage` in place of its separate race-test step. This runs the default suite
with race detection, atomic coverage counters, uncached test execution and `-coverpkg=./internal/...`.
Only application packages under `internal/` contribute to the coverage denominator, including
domain, application, adapters, platform, composition and build metadata. Commands under `cmd/`,
migration tooling under `migrations/` and test infrastructure under `tests/` are excluded. The default
suite still runs across `./...`; cross-package calls contribute coverage. Generated mocks end in
`_test.go` and are naturally outside the production statement denominator.

The minimum remains **64%**, declared in `.coverage-threshold`. This is a statement-weighted total
for the application scope, not an average of package percentages. The measured application-only
baseline is **78.04%**.
`scripts/check-coverage.sh` compares underlying covered/total statement counts before rounding;
repeated blocks from different test binaries are counted once, with hits combined; coverage below
the minimum fails CI. Change the policy file deliberately as meaningful tests improve
coverage; do not lower it or exclude code just to pass CI. Coverage does not establish assertion
quality, branch coverage or correctness, and 100% is not a goal by itself.

Reports are generated under ignored `coverage/`: `coverage.out` for Go tooling, `functions.txt`
for per-function percentages, and `index.html` for source-level inspection. Open index.html locally
or download the CI `coverage` artifact (14-day retention), including when the threshold fails.
`make coverage-check` checks an existing profile without rerunning tests. Test failures also fail
the coverage target; reports from a failed test run may be incomplete.

This gate measures the default suite only. Tagged PostgreSQL integration tests and separate-process
e2e coverage are not merged into the percentage. They remain independent CI checks. Normal unit
tests, make check and pre-commit stay unchanged; local coverage needs no Docker or provider credentials.

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

PostgreSQL stays externally managed: CI owns its service container and local development uses
Compose. Each independent integration or PostgreSQL e2e scenario creates a uniquely named database
through `tests/testdb.CreateDatabase`, applies real application migrations and drops it in cleanup.
Use a dedicated test server; the administrative role needs database creation/removal privileges.
Never supply production or shared business-database credentials.

```sh
docker compose --profile database up -d --wait postgres
export TEST_DATABASE_ADMIN_DSN='postgres://app:local-only-password@localhost:5432/postgres?sslmode=disable'
make test-integration
E2E_DATABASE_ADMIN_DSN="$TEST_DATABASE_ADMIN_DSN" make test-e2e
```

These administrative URLs connect to an existing maintenance database, not the database under test.
The helper returns a DSN targeting the newly created scenario database. Register pool and service
cleanup after creating the database: Go runs cleanup callbacks in reverse registration order.
Database cleanup uses an independent deadline and reports failures; connections must close before
DROP DATABASE. The external container remains running until its owner removes it.

Integration checks apply/checksum migrations, verify idempotence, real unique constraints,
repository round trips, cancellation, transaction rollback and configured pool limits. Missing
TEST_DATABASE_ADMIN_DSN fails explicitly. E2E_DATABASE_ADMIN_DSN selects PostgreSQL-backed
production-mode process tests; leaving it unset retains memory e2e. CI runs both modes. The old
TEST_DATABASE_DSN and E2E_DATABASE_DSN variables are replaced by these explicit administrative URLs.

Related assertions may share one scenario database (for example create, retrieve and duplicate
rejection). Independent scenarios involving lists, counts, pagination or broad mutations must use
separate databases; unique records and eventual deletion alone do not isolate concurrent tests.
Use t.Parallel only for independent scenarios. Targets cap test parallelism at four; separate go test
processes each have their own limit and must also be bounded by the invoking runner. They can share
the same server because generated database names do not collide. CPU, connections and cluster-wide
settings remain shared. Tests do not start containers and require no Testcontainers dependency.

The isolation regression exercises concurrent databases using identical table/record names and
checks that cleanup removes both databases. E2E cleanup waits for the child process to exit before
its database is dropped, including assertion failures. The existing lifecycle scenario builds one
binary and uses one process/database for its related assertions; no speed improvement is claimed
without measurement.

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
