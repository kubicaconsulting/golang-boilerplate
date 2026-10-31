# Configuration

Deployed processes use typed defaults plus independent APP_* environment variables. They never
read YAML. APP_ENV (or --env, which takes precedence) is a deployment label: production, staging,
preview-123 and other lowercase labels are supported. It is not a configuration profile selector.
Only the exact labels local/test enable local conveniences. APP_SERVICE_ENVIRONMENT cannot override
this selector. Choose deployment labels explicitly in your process manager; default execution is local.

In local/test only, precedence is defaults -> optional base.yaml -> optional <environment>.yaml ->
APP_* variables. --config-dir selects the directory (default configs); an empty string disables
files. Missing files are allowed; existing files must contain one document with known fields.
The application and host Air do not automatically load .env. Local YAML supplies the public local
API credential; without files, explicitly configure authentication even in local/test.

| Field | Environment variable | Format/default |
|---|---|---|
| http.address | APP_HTTP_ADDRESS | host:port; :8080 in deployments |
| database.driver | APP_DATABASE_DRIVER | memory or postgres; explicit in deployments |
| database.dsn | APP_DATABASE_DSN | PostgreSQL connection string |
| database.max_conns | APP_DATABASE_MAX_CONNS | integer, 10 |
| database.migration_timeout | APP_DATABASE_MIGRATION_TIMEOUT | duration, 30s |
| logging.level | APP_LOGGING_LEVEL | debug/info/warn/error |
| logging.format | APP_LOGGING_FORMAT | json (deployment default), text or console (local YAML default) |
| logging.color | APP_LOGGING_COLOR | auto (terminal only) or never; console only |
| auth.api_keys | APP_AUTH_API_KEYS | JSON array of key descriptors |
| flags.values | APP_FLAGS_VALUES | JSON object of boolean defaults |
| flags.sdk_key | APP_FLAGS_SDK_KEY | secret string |
| telemetry.enabled | APP_TELEMETRY_ENABLED | boolean |
| shutdown.drain_timeout | APP_SHUTDOWN_DRAIN_TIMEOUT | duration, 10s |
| shutdown.cleanup_timeout | APP_SHUTDOWN_CLEANUP_TIMEOUT | duration, 5s |
| shutdown.telemetry_timeout | APP_SHUTDOWN_TELEMETRY_TIMEOUT | duration, 5s |

Other exported settings use their YAML field names joined by underscores and prefixed APP_.
Durations use Go syntax; complex values use JSON. Environment maps/lists replace the entire value;
YAML maps overlay defaults. See .env.example. The old APP_SHUTDOWN_TIMEOUT setting is replaced by
the three independent budgets above.

PostgreSQL requires a DSN and valid pool bounds; JWT requires issuer/audience and exactly one key
source; LaunchDarkly requires its SDK key. At least one authentication strategy is mandatory.
The public development key is rejected outside local/test. The users composition root separately
rejects memory persistence outside local/test; production therefore requires PostgreSQL until
another durable repository is implemented. This is a feature requirement, not a rule that every
future stateless microservice must have a database. Documentation endpoints default off in deployments.

Secrets come from deployment environment/secret managers; no vendor client is assumed. Redacted
configuration uses an allowlist. Exporters additionally honor standard OTEL_EXPORTER_OTLP_* variables.

## Runtime updates

SIGHUP reloads and validates the complete candidate. Only logging.level and flags.values may change;
static changes reject the whole reload. Local/test YAML edits can supply new dynamic values.
Deployed processes reload their existing environment; changing the parent shell cannot change a
running process. Restart the process for changed deployment environment variables. LaunchDarkly
streamed evaluations update independently; local flag values remain fallback defaults.

Flag maps use copied atomic snapshots; log levels use slog.LevelVar. Updates are not a transaction
across an in-flight request. Evaluate a behavioral flag once at a use-case decision point.
Listeners, databases, credentials, log format/color and provider topology require restart.

Compose reads .env for interpolation and forwards explicitly listed variables. Update that allowlist
when adding container settings; see development.md.
