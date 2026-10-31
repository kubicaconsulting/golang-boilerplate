# 0001: Feature-oriented clean architecture with optional adapters

Status: accepted.

Use domain/application/adapters per feature, Gin routing over a standard net/http server with
Huma code-first OpenAPI, manual constructor wiring and composition-root resource ownership. PostgreSQL is a reference
adapter; memory mode runs without infrastructure. Startup does not migrate implicitly.

Use slog plus OpenTelemetry/OTLP for all signals. New Relic is a destination configuration,
avoiding duplicate native-agent instrumentation. LaunchDarkly is behind an application port
with deterministic local fallbacks. Authenticate API keys/JWT into a common principal, then
authorize use cases independently of transport.

Tradeoffs: more explicit wiring than a service locator; generated OpenAPI couples HTTP DTOs to
Huma; memory mode is non-durable; the small migration runner supports transactional SQL only.
Live reload initially supports log threshold/local flag defaults; resource/credential replacement
requires restart. These constraints keep behavior testable and lifecycle ownership visible.

Gin routing was selected in the 2026-11-01 follow-up using Huma's official humagin adapter.
Keep framework types at the HTTP boundary and process bootstrap. Existing net/http middleware owns
logging, recovery and tracing; the application owns server startup and shutdown. Gin's default
logger/recovery and Run methods are not used. See architecture.md and dependencies.md for the
compatibility decisions and reviewed framework dependency surface.
