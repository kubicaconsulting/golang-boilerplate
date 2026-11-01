# Go implementation rules

Apply the root code-clarity rules and the human review checklist in docs/code-quality.md.

- Run make fmt for gofmt/goimports; make check validates config, formatting and all declared linters.
  Follow docs/code-quality.md. Never disable a compatible checker just to hide findings; any exception
  needs minimal scope, a specific checker and an explanation.
- Domain code imports only the standard library. Application code imports its domain
  and declares the narrow ports it consumes. Application code may also import the SDK-free
  platform/featureflags/definitions package for flag key/fallback constants. No transport/provider
  types in these layers. Declare flag keys and fallbacks there, reference named constants in use
  cases and configuration defaults, and keep deployment overrides in configuration. Never import
  the concrete featureflags provider or SDK into application code. Keep ports narrow: request only
  the evaluation methods the feature consumes. Validate JSON business schemas after decoding, handle
  Enum/JSON definition errors, and use fresh fallback slices instead of mutable global defaults.
  File provider mode must use the official SDK source with events/diagnostics disabled; invalid
  initial files fail startup and clients must close. Keep SDK file paths static until restart.
- Only internal/app selects concrete adapters and owns process resources. Never pass a
  shared Server/config/client bag to a use case; inject its actual dependencies.
- Use domain terms for business names and short, conventional names where the scope makes their
  meaning clear. Prefer a specific name such as signingErr when distinguishing failures; do not
  replace every err with a generic longer synonym merely to satisfy a checker.
- Prefer guard clauses and a readable success path. Keep related declarations together, use natural
  multiline literals, and split strings at semantic boundaries rather than in the middle of words.
- Create interfaces at actual consumer boundaries. Prefer concrete types within an implementation;
  do not add one-method forwarding layers or separate files for every declaration by default.
- Extract a helper when it names a coherent operation, hides a real detail or removes meaningful
  duplication. Do not fragment a readable function solely to reduce line counts or complexity scores.
- Use a blank line between top-level declarations, explicitly including consecutive struct/interface
  type declarations. Keep each doc comment attached to its declaration. Separate validation, acquisition, business work
  and cleanup phases. Keep related assignments and immediate error handling together. Add concise
  comments for non-obvious ownership, dependency order, concurrency and deadline behavior.
- The lint gate includes modernize. Prefer slices.Backward for reverse slice iteration and maps.Copy
  for simple map copying when semantics match; do not apply fixes blindly across mutation or ownership boundaries.
- Keep comments concise and useful; document exported contracts, invariants and surprising decisions
  rather than repeating names or implementation steps.
- Name every project-owned constructor explicitly as New<Type>, such as users.NewUser,
  application.NewService, auth.NewAuthenticator and httpserver.NewServer; do not use bare New.
  Apply this across domain, application, adapters and platform code. Keep standard-library and third-party API names unchanged.
- Use declared package names by default. Let goimports retain an explicit matching import name when
  a directory and its declared package differ. Use users for the users
  domain; alias it userdomain only in files with an actual import or local-name conflict. Apply the same
  rule to future domains: use short lowercase aliases only when a collision or shadowing requires one,
  not merely to label architecture layers or make all imports look alike.
- Use explicit constructors where initialization or dependency injection needs them; use zero values
  when sufficient. Do not add package-global clients or init-time side effects.
- Pass context.Context first through use cases, repositories and outbound operations.
- Use Gin with Huma's humagin adapter in HTTP code only. Keep native Gin types out of domain/use cases.
  Preserve the existing net/http logging, recovery and telemetry layer; do not attach Gin default
  logger/recovery middleware or start an additional listener with Gin Run methods. Select Gin diagnostic
  mode once in process bootstrap so OpenAPI output and service logs remain clean.
- Allocate fresh request DTOs for each request; Huma does this for registered operations.
- Authorize use cases independently of HTTP authentication. Never trust caller-supplied roles.
- Translate known database constraints inside adapters; keep API error contracts independent
  of table names. Public unexpected errors must be generic.
- Keep stable component attributes structured in every sink. Console colors belong only in the
  terminal handler; never put ANSI codes in messages or attributes. Log initialization milestones in
  the composition root only after successful construction, and readiness only after binding the listener.
- Use structured context-aware logs. Do not log credentials, raw bodies, email addresses or
  raw query strings. Do not use user IDs/request IDs as metric labels.
- Every acquired resource needs failure-path cleanup and lifecycle registration. Cleanup
  continues after an error and honors deadlines. HTTP drains before persistence closes.
- Validate a complete reload before applying supported dynamic settings. Static resources
  and credential changes require restart.
- Keep OpenAPI metadata beside routes, DTO validation beside transport fields, and domain
  invariants in domain constructors. Keep errors/examples accurate.
- Use Testify require/assert for readable tests and Mockery for selected consumer-owned interfaces.
  Configure generation explicitly in .mockery.yml; keep mocks in feature-local mocks_test.go files
  under external test packages. Run make mocks after interface changes and make mocks-check before
  finishing. Never edit generated mocks or import testing libraries into production code. Prefer
  feature-local helpers for repeated setup; keep trivial handwritten fakes when they are clearer.
- Use generated mocks or narrow fakes for application tests, real PostgreSQL for persistence semantics, and
  real-process e2e for signals. See docs/testing.md.
- `internal/architecture/boundaries_test.go` enforces the current users boundary; extend it
  when introducing features rather than weakening existing rules.

- Dependency selection must follow docs/dependencies.md. Review upstream ownership and the
  transitive graph before adding packages; prefer official SDKs and established projects.
