# Contributing

Use the Go version pinned in CI and Dockerfile. Begin with `go mod download` and `make check`; use `make dev` for Air or `make docker-dev` for container development.
`make fmt` formats code; `make lint` uses the exact linter version in Makefile. Dependencies
are pinned in go.mod/go.sum; tool versions are pinned in Makefile.

Run `make hooks-install` once after cloning to activate the tracked `.githooks/pre-commit` hook.
Every commit runs `make lint`, `make test-unit` and `make openapi`; any failure blocks the commit.
If the regenerated `api/openapi.json` differs from the staged version, the hook stops so you can
review it, run `git add api/openapi.json`, and retry the commit. It never stages files automatically.
The hook checks the current working tree, including unstaged edits, and does not format Go source.
With partially staged changes, keep unstaged changes compatible with the staged code. Go and Make
must be available in the committing process's PATH, including when committing through an IDE.
Hook activation is local to each clone; CI continues to run the full quality gate independently.

For a new feature, start with domain rules and application tests, then add only the adapters
it needs. Register resources in internal/app. Follow internal/AGENTS.md and the architecture
guide. Keep changes reviewable and explain behavior, verification and material limitations.

Write clean, simple, idiomatic Go with domain-based naming and the readability of code carefully
written by a senior engineer. Follow [Effective Go](https://go.dev/doc/effective_go) and the
[human review checklist](docs/code-quality.md#human-readability-and-idiomatic-go). Prefer the smallest
change and file structure that expresses the behavior clearly. Avoid speculative abstractions,
pass-through layers, unnecessary files and mechanical formatting that makes code harder to read.
Readability review is required alongside lint checks.

A behavior change includes tests and documentation. HTTP changes regenerate OpenAPI; database
changes add a migration and run integration tests; lifecycle changes run e2e tests. Run
`make test-race` before submitting Go changes. Run `make vuln` when changing dependencies.
Do not add tests that merely repeat implementation details or demand external SaaS accounts.

The Go module path is `boilerplate`. When creating your real service, replace
that path consistently in go.mod, imports and the architecture-boundary test. Rename service
identity in configs/base.yaml. Review public operations/scopes and local defaults before deploy.

Dependency changes follow [the dependency policy](docs/dependencies.md). Review the entire module
graph diff before `make deps-inventory`; do not automatically approve unknown modules. Run
`make docker-config` for Compose changes and build both Dockerfile targets for container changes.
