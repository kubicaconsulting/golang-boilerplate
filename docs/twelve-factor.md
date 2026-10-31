# Twelve-factor alignment

This template supports the [Twelve-Factor App](https://12factor.net/) practices. Some factors depend
on how the consuming service is deployed; the template cannot certify an unknown deployment.

| Factor | Implementation / deployment responsibility |
|---|---|
| Codebase | Initialized Git repository; review and create the first commit, configure your remote. |
| Dependencies | Go modules/checksums, reviewed graph, explicit pinned tools and container build. |
| Config | Deployed environments use independent environment variables; local/test YAML is optional. Secret injection belongs to the deployment. |
| Backing services | PostgreSQL, OTLP and LaunchDarkly are configuration-selected adapters; users require durable storage in deployments. No email/cron/queue assumption. |
| Build/release/run | Embedded metadata, clean-tree unique release tags, CI image archive. Choose registry retention/tag protection and promote the same digest. |
| Processes | No required local state in deployment; durable users live in PostgreSQL. Memory repository is local/test only. |
| Port binding | Self-hosted HTTP listener; explicit address, deployment probes and gateway policy. |
| Concurrency | Run independent service replicas; bound aggregate pool sizes. Load balancing/autoscaling are deployment decisions. |
| Disposability | Signals mark unready, drain, clean resources and flush telemetry with separate budgets. Termination grace is deployment-owned. |
| Dev/prod parity | Optional PostgreSQL 17 workflow matches CI; test persistence with the actual chosen production major/version and extensions. |
| Logs | Structured stdout event stream plus optional OTLP; route/store externally. No application log files. |
| Admin processes | Explicit migration command uses the same binary/configuration, with independent timeout. |

Local convenience modes deliberately include a public credential and non-durable repository; never
use local/test deployment labels for real environments. Production/custom labels ignore all YAML.
See configuration.md, operations.md and releases.md for concrete boundaries and remaining operator work.
