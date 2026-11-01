# Repository instructions

Read README.md and the relevant docs before changing behavior. These instructions apply
throughout the repository; internal/AGENTS.md adds Go implementation rules.

## Authoritative references

- Architecture and dependencies: docs/architecture.md.
- Configuration and reload policy: docs/configuration.md.
- Public API and generation: docs/api.md; Go route declarations/DTOs are authoritative.
- Authentication, observability and flags: their corresponding docs/ guides.
- Development checks: Makefile and docs/testing.md.
- Formatting/lint policy and exception rationale: .golangci.yml and docs/code-quality.md.
- Dependency selection and approval: docs/dependencies.md; inventory drift is checked in CI.

## Code clarity and simplicity

Write clean, simple code that reads like carefully maintained, handwritten Go from a senior engineer.
These requirements apply to generated and edited code alike; passing linters is only part of review.

- Use the domain language for business types, operations and variables. Choose names that explain
  their role in context; avoid vague names, invented terminology and redundant package/type prefixes.
- Use each imported package's declared name by default. Add a short lowercase alias only for a real
  name collision or shadowing conflict; do not rename imports merely for uniformity. For the users
  domain, use `users` by default and `userdomain` only where a conflict requires it.
- Name project-owned constructors `New<Type>` across all layers (for example, `NewUser`, `NewService`,
  `NewRepository` and `NewServer`), rather than bare `New`. Keep external library API names unchanged.
- Prefer straightforward control flow, explicit dependencies and idiomatic Go. Follow
  [Effective Go](https://go.dev/doc/effective_go) and the guidance in docs/code-quality.md.
- Keep code human readable: natural statement layout, cohesive functions and useful comments about
  intent, invariants or tradeoffs. Avoid mechanical wrapping, cryptic abbreviations and comments
  that merely narrate syntax.
- Separate functions, type declarations (including structs and interfaces), and distinct units of work
  with a blank line. Keep doc comments directly attached to their declaration. Keep error checks adjacent to
  the operation they check. Add comments where intent, ordering, ownership or a tradeoff is not
  obvious; do not narrate every statement or pad every block.
- Review modernization diagnostics against the module's supported Go version and behavior; apply
  clear standard-library improvements and verify that ordering, cancellation and contracts remain intact.
- Add only the files, packages, types and abstractions needed for the current behavior or a real
  boundary. Reuse an appropriate existing file; do not generate empty layers, speculative extension
  points, pass-through wrappers or an interface for every struct.
- Simplify before adding machinery. A little clear repetition can be better than premature generic
  infrastructure. Preserve architecture boundaries, validation, authorization and resource cleanup.
- Before finishing, read the diff as a maintainer: can its purpose and failure paths be understood
  without following unnecessary indirection, and can anything be removed without losing clarity?

## Foundation and extension policy

Keep the baseline focused on clear feature boundaries, explicit constructor injection, validated
configuration, authentication/authorization, resource ownership, observability, API contracts and
meaningful automated checks. Prefer the smallest implementation that preserves those guarantees.

Before introducing a pattern, identify the current requirement or failure mode, explain why the
existing direct approach is insufficient, and verify the behavior that the pattern protects. A real
consumer boundary can justify an interface with one implementation; hypothetical future reuse cannot.
Apply patterns within the feature or adapter that needs them before considering shared infrastructure.
See docs/architecture.md for concrete extension triggers. Do not expand the baseline merely to match
patterns or infrastructure present in another service.

## Completion criteria

1. Keep changes within the requested capability; do not assume email, cron, queues or Redis.
2. Add meaningful behavior tests, including relevant failure and cancellation paths.
3. Run `make check` and `make test-race` for Go changes.
4. For persistence changes, run `make test-integration` with TEST_DATABASE_ADMIN_DSN targeting a dedicated test PostgreSQL server.
5. For HTTP/lifecycle/auth changes, run `make test-e2e`.
6. For public API changes, run `make openapi`, review the spec diff, then `make contract-check`.
7. Update Markdown documentation in the same change as behavior.
8. Report exact checks run and any unavailable dependency; never describe skipped tests as passed.

Never commit secrets or manually edit api/openapi.json. Never amend an applied migration;
add a new migration. Do not add unimplemented features to the README. Provider credentials
must not be needed by normal tests. Document significant design decisions in docs/decisions/.

## Dependencies and developer tooling

Use standard-library implementations first, then official SDKs or established widely used
projects. Review direct and indirect provenance before adding/updating packages. Do not use
small unknown wrappers when an official/popular library already provides the capability.
Follow docs/dependencies.md; never blindly regenerate dependencies.lock.json to pass a check.
Keep Air development-only and its pinned version consistent across Makefile/Dockerfile/Compose.
Host Air intentionally does not load .env. Update development/docs when changing watch or
container configuration, and verify both production/dev builds when Docker is available.

## Deployment and release invariants

- Non-local/test labels load environment variables only; do not add deployment YAML profiles.
- Keep the users durable-storage guard in composition wiring; do not require a database for every service.
- Preserve independent HTTP drain, infrastructure cleanup and telemetry flush budgets, including failures.
- Keep migration timeout independent. Adjust Air/Compose grace when shutdown budgets change.
- Build metadata must match the binary and OCI labels. Release from clean committed source, use unique
  IDs, and promote the same image digest; never commit or publish on the user's behalf without authorization.
- Keep the zero-infrastructure default and optional PostgreSQL parity path documented and tested.

## Test database isolation

Keep PostgreSQL container ownership in CI or local Compose. Use tests/testdb.CreateDatabase for
each independent integration or PostgreSQL e2e scenario; related assertions may share that database.
Unique record IDs alone do not isolate lists, counts, pagination or broad mutations. Apply real
application migrations and register service/pool cleanup after database creation so resources close
before the database is dropped. Never use production/admin service credentials in normal application
configuration. Keep container-dependent suites separate from make check and pre-commit checks.

## Coverage policy

CI enforces default-suite statement coverage through make test-coverage and .coverage-threshold.
Coverage includes application packages under internal/; cmd/, migrations/ and tests/ are outside
the denominator. Keep all application packages in scope; do not lower the threshold or add further
exclusions to mask missing tests. Prefer meaningful behavior and failure-path assertions. Increase the floor deliberately as
coverage improves. Integration and subprocess e2e coverage are separate from this default-suite gate.
