# Observability

`slog` writes structured JSON by default in deployments. Local development uses a console layout;
tests retain plain text. `APP_LOGGING_FORMAT` accepts `json`, `text` and `console`. Configure debug/info/warn/
error levels; SIGHUP reloads the threshold. HTTP logging records the final status, elapsed time,
method and generated request ID, without raw URI/query, credentials, bodies or email addresses.
Active trace/span IDs are added automatically to context-aware logs.

Set APP_TELEMETRY_ENABLED=true to enable OTLP/HTTP protobuf exporters for logs, metrics and
traces. Disabled mode opens no exporter connection. Providers use the same service.name,
service.version and deployment.environment.name. Parent-based trace sampling uses the configured
sample_ratio. HTTP metrics/runtime metrics and database query spans are provided. SQL statements
and query arguments are not attached to database spans. Route templates keep metric cardinality
bounded; liveness/readiness are excluded from HTTP telemetry. Providers flush after requests drain.

```sh
docker compose --profile telemetry up -d collector
APP_TELEMETRY_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 make run
```

The optional collector profile prints signal summaries. An automated local receiver test verifies
all three OTLP signals and log/trace correlation. Live exporter delivery is not required for readiness;
exporters have bounded queues, export timeouts and shutdown deadlines. Their asynchronous errors
are reported by the SDK rather than terminating request handling.

## Development console and initialization events

Console output shows a timestamp, level, optional `[Component]`, message and structured attributes:

```text
10:42:01.123 INFO  [Database] persistence initialized driver=memory duration=2µs
10:42:01.124 INFO  [Users] users feature initialized duration=3µs
10:42:01.125 INFO  [Application] service ready address=127.0.0.1:8080
```

`APP_LOGGING_COLOR=auto` is the default. Only console output connected to a terminal gets colored
levels. Redirected output, `NO_COLOR` with a nonempty value, `TERM=dumb`, or `APP_LOGGING_COLOR=never`
disables ANSI colors. JSON and text output never add colors. Console escaping keeps control characters
in messages and attributes from injecting terminal commands or extra log lines. Color selection and
format require restart; level changes still support SIGHUP.

The composition root emits successful initialization events for observability, persistence,
authentication, feature flags, users and HTTP routes, with component durations. The final application
initialization duration measures construction; `service ready` is emitted only after binding the
listener. These are explicit wiring milestones, not automatic module discovery. Failures still unwind
resources and do not emit success messages for the failed component. Memory persistence reports
`driver=memory`, without implying a database connection was opened.

Attach a stable `component` attribute when injecting a logger, for example
`logger.With("component", "Users")`. Component names and timing attributes remain structured in JSON
and OTLP; terminal styling is applied only by the console sink. Domain and application use cases do
not need a logging framework or provider dependency.

## New Relic

Use the same OTLP pipeline, without a parallel native agent:

```sh
export APP_TELEMETRY_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.nr-data.net
export OTEL_EXPORTER_OTLP_HEADERS="api-key=<your-license-key>"
export OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=delta
make run
```

Choose the correct New Relic region endpoint. Keep the header in your secret manager and never
commit it. Standard signal-specific OTEL_EXPORTER_OTLP_* overrides are also supported. Prefer an
OpenTelemetry Collector when centralizing retries, redaction and routing. If a collector already
ships stdout, disable its duplicate log route when direct OTLP log export is enabled.

See [New Relic OTLP guidance](https://docs.newrelic.com/docs/opentelemetry/best-practices/opentelemetry-otlp/).
Never label metrics by request ID, subject, email or arbitrary URLs. Provider-specific instrumentation
must stay outside domain/application packages.
