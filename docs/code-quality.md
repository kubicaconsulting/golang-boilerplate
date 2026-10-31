# Declarative code quality policies and migration report

Verified migration: 2026-10-31. Source: reference apps/backend/.golangci.yml; target: this repository.
The reference repository was read without modification. The executable authority is .golangci.yml,
validated against pinned GolangCI-Lint v2.15.0, rather than a prose-only checklist.

## Human readability and idiomatic Go

Code must be clean, simple and easy for another engineer to maintain. Write it with the care expected
from a senior engineer working by hand: deliberate names, direct control flow and only the structure
that the problem needs. Automated checks support this standard; they cannot establish readability
or justify unnecessary abstractions.

Use the product's domain language consistently in business code. For the reference feature, User,
email uniqueness and permission scopes should mean the same thing in use cases, adapters and docs.
Name technical plumbing by its actual responsibility. Avoid vague names such as data, manager or
processor when a more precise term exists. Conventional short names such as ctx, err and a local
loop index are appropriate when their meaning is obvious; longer names are not automatically clearer.

Follow [Effective Go](https://go.dev/doc/effective_go) for core language idioms: naming in package
context, standard formatting, useful documentation, interfaces and straightforward error handling.
It is a foundational guide rather than a complete account of modern Go; consult the official language
and library documentation for the Go version used by the project.

Keep the successful path easy to follow, handle errors close to the operation and avoid deep nesting.
Use zero values where they work and constructors where they establish invariants or inject dependencies.
Name every project-owned constructor explicitly as `New<Type>`, rather than bare `New`. Apply this
across all layers: `users.NewUser`, `application.NewService`, `memory.NewRepository`, `userhttp.NewHandler`,
`auth.NewAuthenticator`, `featureflags.NewProvider`, `httpserver.NewServer` and `app.NewRuntime`.
Standard-library and third-party constructors keep their own names. Operations such as configuration
`Load` and database `Open` retain verbs that describe their work.

Use each imported package's declared name by default. When a directory and its declared package name
differ, retain the explicit matching import name emitted by goimports (such as `users` for the `domain`
directory). For the users domain,
write `users.NewUser` and `users.User`; use `userdomain` only in a file where another import or a local
name such as a `users` slice or parameter creates a conflict. Apply the same rule to future domains.
Aliases should be short and lowercase, without underscores or mixed case. Do not alias imports solely
for uniformity or to repeat the architecture layer. Struct fields such as `repository.users` do not
shadow an imported package by themselves.

Write natural multiline calls and literals. Break long SQL or strings at meaningful boundaries, never
mechanically inside words or identifiers. Comments should explain contracts, intent and non-obvious
constraints, without restating each line of code.

Use the [foundation and extension policy](architecture.md#foundation-and-extension-policy) when
introducing patterns: name the current requirement, justify the added structure and verify its benefit.

Keep the project small. Add a file or package for a cohesive responsibility, a real dependency boundary
or navigability, rather than to satisfy a diagram. Related small declarations can share a file. Add only
adapters the feature actually uses. Avoid empty directories, speculative frameworks, generic CRUD bases,
forwarding wrappers and automatic interface/implementation pairs. Introduce an abstraction when a current
consumer, invariant or meaningful repetition justifies it; simple duplication can be clearer than a
premature abstraction. Preserve the established dependency direction and operational guarantees.

When a style or complexity check encourages awkward code, first look for a coherent simplification.
Do not scatter declarations, invent meaningless helpers or shorten names just to meet a metric. If the
clearest correct implementation still needs an exception, document the narrow checker-specific reason
under the existing exception policy. Do not silently weaken validation, security or lifecycle checks.

Before submitting code, review it as a human maintainer:

- Do names describe domain concepts and responsibilities consistently?
- Can the success path, errors and resource ownership be understood directly?
- Does each helper, interface, package and file earn its place through a present need?
- Are formatting and comments natural, with no mechanical string splitting or filler explanations?
- Can any scaffolding be removed while preserving behavior, boundaries and testability?

## Modernization, comments and spacing

The pinned GolangCI-Lint modernize suite is enabled with no disabled analyzers. It checks the module's
Go language version and runs with the same integration/e2e tags as the other linters. The baseline
has 68 checks from the reference migration; modernize brings the current total to **69 linters**.
Review suggested changes for behavior, ownership and clarity rather than applying them blindly.

The initial sweep found two diagnostics. Resource cleanup now uses slices.Backward, preserving reverse
registration order and the original slice index used to budget remaining cleanup work. Local flag
snapshots use maps.Copy into a newly allocated map, preserving independence from caller mutations and
keeping nil input safe. No runtime dependency is added for either standard-library operation.

Use a blank line between functions and other top-level declarations, including consecutive type,
struct and interface declarations. Keep doc comments directly attached to the declaration they describe.
Also separate distinct phases
such as validation, resource acquisition, business operations and cleanup. Keep related assignments
and the immediate error check together. Avoid blank lines after every statement or at block edges.
Comments should explain non-obvious reasons, dependency order, lifetime, synchronization and deadlines;
self-explanatory assignments do not need narration. Formatting and lint catch only part of this rule:
review the spacing and comments as a human maintainer too.

## Readability refactor applied

The code review applied the above guidelines to the existing implementation and tests:

- User values, actors, authentication principals, repository results and error variables now use
  names that explain their domain or operation, such as user, users, principal and scanErr.
- Composition wiring names the repository, authenticator, users service and HTTP handler directly.
  Their ownership and startup cleanup remain in the composition root.
- Removed unused private clock/ID function fields; user creation uses the standard-library clock
  and secure identifier generator directly. Real repository/flag interfaces remain consumer-owned.
- Compact small metadata literals and simple calls, group structured log keys with their values,
  and keep multiline layout for larger configurations. Expanded multi-statement callbacks and
  test buffer methods into normal blocks. Test cases remain one scenario per row.
- Named the remaining-resource/signal budgets in cleanup loops and removed redundant intermediate
  contexts, preserving independent phase deadlines and failure handling.
- Kept all changes in existing files; no new runtime packages, dependencies or forwarding layers.

These changes are readability improvements, not a statement that lint can measure human authorship.
The validation record is in verification.md. Public HTTP contracts and generated OpenAPI stay stable.

## Applied files and commands

| File | Responsibility |
|---|---|
| .golangci.yml | Explicit enabled checks, thresholds, formatter selection, build tags, dependency bans and justified exclusions. |
| .editorconfig | UTF-8/LF/final newline, Go tabs displayed at width 2, Make recipe tabs, two-space general indentation, preserved Markdown hard breaks. |
| .gitattributes | Normalize text to LF and identify common binary assets. |
| Makefile | Pinned tool, schema validation, read-only format drift check, lint and optional automatic fixes. |
| Taskfile.yml | Matching fmt, format:check, lint, lint:fix and lint:config aliases. |
| .github/workflows/ci.yml | Existing make check step now enforces the expanded configuration and format checks. |
| AGENTS.md and internal/AGENTS.md | Require the declarative policy and explained, narrow exceptions when agents change code. |

Go source retains literal tabs from gofmt/goimports; .editorconfig displays each tab at two columns.
This is an editor display preference, not conversion to spaces. Make recipe indentation retains tabs.

make fmt applies gofmt and goimports. make format-check compares formatter output and fails on any
diff without rewriting files. make lint-config-check validates schema/tool compatibility. make lint
runs all enabled policies; make lint-fix applies available automatic fixes and should be reviewed.
make check includes config validation, formatting, module inventory, vet, lint, tests and OpenAPI drift.
Both integration and e2e build tags are enabled for lint analysis; execution remains in their separate
commands with explicit database configuration. Provider accounts are not needed for static checks.

## Applied policy summary

The reference declared 72 linter names. Of those, 67 remain enabled under the same names. Five names
are migrated or removed as described below. Adding depguard produced a baseline of **68 linters**, plus **two
formatters**; the modernization follow-up brings the current total to **69 linters**. No compatible enabled reference check was dropped outright. Some checks have narrow,
explicit exceptions; they must not be described as universally enforced over excluded code.

- Correctness, security and resource lifecycle: checked errors/type assertions, vet, staticcheck,
  gosec, HTTP body closure, SQL rows/closure, contexts, nil handling, spans and exhaustive cases.
- Architecture/style: no runtime globals or init functions, structured/context-aware logs,
  maintained dependency imports, API comments/tags, method receiver and interface consistency.
- Maintainability: function size, duplication, cyclomatic/cognitive complexity, nesting, line
  length, constants, numeric intent and explained linter suppressions.
- Tests/performance: external test package preference, parallel-test consistency, examples,
  assertion guidance, protobuf getters, conversions and allocation/string checks.

Thresholds retain cyclop max 30/package average 10, gocognit 20, funlen 100 lines/50 statements
excluding comments, 120-column lines, and no naked returns. errcheck checks type assertions; govet
enables its analyzers with strict shadow detection and retains the reference fieldalignment opt-out.
The reference’s implicit defaults are now explicit: gocyclo 30, nestif 5, dupl 150 tokens, and lll
120 columns/tab width 1. Slog forbids global loggers and requires context when one is in scope. All nolint directives now
require an explicit checker name and an explanation, including the three reference waiver categories.
All diagnostics are reported: reference max-same-issues=50 becomes unlimited, with no per-linter cap.
The run timeout is 5m rather than 3m to allow the expanded tagged analysis on CI.

## Compatibility changes and omitted configuration

| Reference item | Resolution / reason |
|---|---|
| gosimple, stylecheck | Removed standalone names; enabled all staticcheck checks, including S/ST families. |
| typecheck | Removed from the enable list; compiler/type failures still fail lint/tests. |
| goimports in linters | Moved to formatters; added explicit gofmt and format drift enforcement. |
| gomodguard | Replaced deprecated checker with depguard bans for github.com/golang/protobuf, github.com/satori/go.uuid and unversioned github.com/gofrs/uuid; retain maintained alternatives in messages. |
| exhaustive check: map | Updated to mapliteral; switch/map exhaustiveness remains enabled. |
| exhaustruct settings | Removed inactive settings and provider-specific allowlist; the reference never enabled this checker. |
| inamedparam settings | Removed inactive settings; the reference never enabled this checker. |
| rowserrcheck sqlx package setting | Removed sqlx-specific configuration because this project uses pgx. rowserrcheck and sqlclosecheck remain enabled. |
| godot TODO/noinspection exclusions | Removed because godot was not enabled. Existing TODO text does not justify bypassing other checks. |
| gocritic //noinspection exclusion | Removed blanket comment-based bypass; use an explained, specific suppression if necessary. |
| wrapcheck test exclusion | Removed because wrapcheck was not enabled in the reference. |
| Broad test bodyclose/errcheck/gosec/noctx exclusions | Not copied; those checks now apply to tests too, except controlled e2e subprocess G204 below. |
| Existing target exclusion presets | Removed broad comments/common-false-positives/legacy/std-error-handling presets; decisions are explicit. |
| nolint explanations optional for funlen/gocognit/lll | Removed waiver; all suppressions need explanations. |

Depguard rejects prohibited imported package paths, while make deps-check detects unreviewed module
version/graph changes and replacements. This differs from banning all matching transitive module
entries: the deprecated gomodguard policy targeted direct dependencies. Inventory review remains
required; configuring import bans is not a claim that every transitive package is risk-free.

## Explicit scope exceptions

| Scope | Exception | Reason |
|---|---|---|
| *_test.go | dupl, funlen, goconst, cyclop, gocognit, gocyclo, nestif, mnd | Scenario fixtures repeat data and assertions; avoid refactoring tests into opaque abstraction merely to meet size/complexity metrics. Production thresholds remain intact. |
| internal/app/drain_test.go, internal/app/shutdown_test.go, internal/platform/featureflags/provider_test.go | testpackage | White-box checks require private lifecycle registration/serving or controlled SDK construction. All other test files retain the external-package policy. |
| internal/buildinfo/info.go | gochecknoglobals | Go linker -X requires package variables; this does not allow runtime client globals elsewhere. |
| cmd/ | forbidigo | Administrative commands deliberately write output; server implementation still forbids ad hoc printing. |
| tests/e2e/service_test.go, G204 only | gosec | The test launches a binary it just built in a private temporary directory, with controlled arguments. Other security checks remain enabled. |
| auth.TestJWKSRejectsIncompatibleDefaultTransport | reassign | A sequential regression test replaces and restores http.DefaultTransport to exercise the checked assertion failure; runtime reassignment remains forbidden. |
| config.Defaults | mnd | Typed named configuration fields declare operational defaults centrally; duplicating each value into a second constant adds no intent. |
| config.loadYAML file open | G304 | The operator explicitly selects local/test paths; deployment labels never open YAML files. |
| config.setEnvironmentValue kind switch | exhaustive | Scalar parsing is explicit; remaining reflect kinds intentionally use JSON replacement. |
| featureflags severity map | exhaustive | ldlog.None disables logging; it is not an emitted severity to map to slog. |
| database.TraceQueryStart | spancheck | pgx splits start/end callbacks; TraceQueryEnd ends the span carried in the returned context. Ending it inside start would break instrumentation. |

No baseline of existing violations or whole-project suppression is used. New exemptions require a
concrete reason, minimal scope and a documentation update.

## Code changes needed to satisfy the policy

- Added meaningful exported API/type comments and explicit JSON decoding tags.
- Replaced context-free listeners, test requests and subprocess launches with context-aware variants.
- Checked the default HTTP transport assertion before creating an isolated JWKS client, avoiding a panic
  if another component replaced the process default with an incompatible RoundTripper.
- Used context-aware logging and the standard slog discard handler; retained structured output.
- Removed shadowed error bindings, used protobuf getters and canonical header spelling, and made
  intentional ignored close errors explicit. HTTP header matching remains case-insensitive.
- Split configuration validation/conversion, JWT setup, route authentication middleware and per-file
  migration work into focused helpers. Migration SQL, ordering, locking and checksums are preserved.
- Named operational timeout/size constants outside the centralized defaults and used HTTP status constants.
- Expanded long declarations/calls and retained generated OpenAPI unchanged (verified by contract drift).
- Dependency inventory writes request 0600 permissions for newly created files; no package was added.

## Complete reference-policy disposition

| Reference name | Disposition | Policy |
|---|---|---|
| `errcheck` | Enabled | Errcheck is a program for checking for unchecked errors in Go code |
| `gosimple` | Merged | staticcheck with checks: [all] covers S-series checks. |
| `govet` | Enabled | Vet examines Go source code and reports suspicious constructs |
| `ineffassign` | Enabled | Detects when assignments to existing variables are not used |
| `staticcheck` | Enabled | It's the set of rules from staticcheck |
| `typecheck` | Removed name | Compiler/type analysis is intrinsic to linting and go test; it cannot be enabled as a linter. |
| `unused` | Enabled | Checks Go code for unused constants, variables, functions and types |
| `asasalint` | Enabled | Check for pass []any as any in variadic func(...any) |
| `asciicheck` | Enabled | Checks that all code identifiers does not have non-ASCII symbols in the name |
| `bidichk` | Enabled | Checks for dangerous unicode character sequences |
| `bodyclose` | Enabled | Checks whether HTTP response body is closed successfully |
| `canonicalheader` | Enabled | Canonicalheader checks whether net/http.Header uses canonical header |
| `copyloopvar` | Enabled | A linter detects places where loop variables are copied |
| `cyclop` | Enabled | Checks function and package cyclomatic complexity |
| `dupl` | Enabled | Detects duplicate fragments of code |
| `durationcheck` | Enabled | Check for two durations multiplied together |
| `errname` | Enabled | Checks that sentinel errors are prefixed with the `Err` and error types are suffixed with the `Error` |
| `errorlint` | Enabled | Find code that can cause problems with the error wrapping scheme introduced in Go 1.13 |
| `exhaustive` | Enabled | Check that expression switches and type switches are exhaustive |
| `fatcontext` | Enabled | Detects nested contexts in loops and function literals |
| `forbidigo` | Enabled | Forbids identifiers |
| `funlen` | Enabled | Checks for long functions |
| `gocheckcompilerdirectives` | Enabled | Checks that go compiler directive comments (//go:) are valid |
| `gochecknoglobals` | Enabled | Check that no global variables exist |
| `gochecknoinits` | Enabled | Checks that no init functions are present in Go code |
| `gochecksumtype` | Enabled | Check exhaustiveness of sum type switch statements |
| `gocognit` | Enabled | Computes and checks the cognitive complexity of functions |
| `goconst` | Enabled | Finds repeated strings that could be replaced by a constant |
| `gocritic` | Enabled | Provides diagnostics that check for bugs, performance and style issues |
| `gocyclo` | Enabled | Computes and checks the cyclomatic complexity of functions |
| `goimports` | Moved | Enabled under formatters, alongside gofmt. |
| `gomoddirectives` | Enabled | Manage the use of 'replace', 'retract', and 'excludes' directives in go.mod |
| `gomodguard` | Replaced | Deprecated checker replaced by depguard import bans plus the existing reviewed module inventory. |
| `goprintffuncname` | Enabled | Checks that printf-like functions are named with `f` at the end |
| `gosec` | Enabled | Inspects source code for security problems |
| `iface` | Enabled | Detect the incorrect use of interfaces, helping developers avoid interface pollution |
| `intrange` | Enabled | Intrange is a linter to find places where for loops could make use of an integer range |
| `lll` | Enabled | Reports long lines |
| `loggercheck` | Enabled | Checks key value pairs for common logger libraries (kitlog,klog,logr,slog,zap) |
| `makezero` | Enabled | Find slice declarations with non-zero initial length |
| `mirror` | Enabled | Reports wrong mirror patterns of bytes/strings usage |
| `mnd` | Enabled | An analyzer to detect magic numbers |
| `musttag` | Enabled | Enforce field tags in (un)marshaled structs |
| `nakedret` | Enabled | Checks that functions with naked returns are not longer than a maximum size (can be zero) |
| `nestif` | Enabled | Reports deeply nested if statements |
| `nilerr` | Enabled | Find the code that returns nil even if it checks that the error is not nil |
| `nilnil` | Enabled | Checks that there is no simultaneous return of `nil` error and an invalid value |
| `noctx` | Enabled | Detects function and method with missing usage of context.Context |
| `nolintlint` | Enabled | Reports ill-formed or insufficient nolint directives |
| `nonamedreturns` | Enabled | Reports all named returns |
| `nosprintfhostport` | Enabled | Checks for misuse of Sprintf to construct a host with port in a URL |
| `perfsprint` | Enabled | Checks that fmt.Sprintf can be replaced with a faster alternative |
| `predeclared` | Enabled | Find declarations that shadow Go's predeclared identifiers |
| `promlinter` | Enabled | Check Prometheus metrics naming via promlint |
| `protogetter` | Enabled | Reports direct reads from proto message fields when getters should be used |
| `reassign` | Enabled | Checks that package variables are not reassigned |
| `recvcheck` | Enabled | Checks for receiver type consistency |
| `revive` | Enabled | Fast, configurable, extensible, flexible, and beautiful linter for Go |
| `rowserrcheck` | Enabled | Checks whether Rows.Err of rows is checked successfully |
| `sloglint` | Enabled | Ensures consistent code style when using log/slog |
| `spancheck` | Enabled | Checks for mistakes with OpenTelemetry/Census spans |
| `sqlclosecheck` | Enabled | Checks that sql.Rows, sql.Stmt, sqlx.NamedStmt, pgx.Query are closed |
| `stylecheck` | Merged | staticcheck with checks: [all] covers ST-series checks. |
| `testableexamples` | Enabled | Linter checks if examples are testable (have an expected output) |
| `testifylint` | Enabled | Checks usage of github.com/stretchr/testify |
| `testpackage` | Enabled | Linter that makes you use a separate _test package |
| `tparallel` | Enabled | Tparallel detects inappropriate usage of t.Parallel() method in your Go test codes |
| `unconvert` | Enabled | Remove unnecessary type conversions |
| `unparam` | Enabled | Reports unused function parameters |
| `usestdlibvars` | Enabled | A linter that detect the possibility to use variables/constants from the Go standard library |
| `wastedassign` | Enabled | Finds wasted assignment statements |
| `whitespace` | Enabled | Whitespace is a linter that checks for unnecessary newlines at the start and end of functions, if, for, etc |

Additional declarations: depguard replaces gomodguard; gofmt is the canonical source formatter.
Architecture-boundary tests and dependency inventory checks remain independent complementary gates.

## Validation

See docs/verification.md for executed checks and limitations. The configuration is validated against
the locally pinned tool, not assumed compatible from its version field. GitHub-hosted CI has not been
run here; its existing make check step inherits these checks when the repository is pushed.
