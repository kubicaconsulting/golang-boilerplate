# Dependency policy and initial audit

Reviewed on 2026-10-31. Prefer Go's standard library. Every direct dependency must be an official
provider/project SDK or an established community project with public ownership, adoption,
maintenance, releases and security reporting. Stars alone are not approval. Avoid obscure
convenience wrappers, abandoned forks, custom cryptography and unchecked replacements.

## Selected direct libraries

| Dependency | Provenance and reason |
|---|---|
| [CoreOS go-oidc](https://github.com/coreos/go-oidc) | Established OIDC verifier, cached JWKS/signature handling; uses maintained go-jose |
| [golang-jwt/jwt](https://github.com/golang-jwt/jwt) | Established Go JWT project; registered-claim validation and RSA PEM parsing |
| [Gin](https://github.com/gin-gonic/gin) | Official gin-gonic release v1.12.0; established router selected by the user; official Huma adapter |
| [Huma](https://github.com/danielgtaylor/huma) | Established code-first OpenAPI project, public users and active releases; approximately 4.5k stars at review |
| [httpsnoop](https://github.com/felixge/httpsnoop) | Established HTTP instrumentation helper also used by OpenTelemetry; preserves ResponseWriter interfaces |
| [pgx](https://github.com/jackc/pgx) | Established PostgreSQL driver and pool; direct SQL without an ORM |
| [LaunchDarkly SDKs](https://github.com/launchdarkly/go-server-sdk) | Official provider SDK and common types |
| [OpenTelemetry](https://github.com/open-telemetry/opentelemetry-go) and [contrib](https://github.com/open-telemetry/opentelemetry-go-contrib) | Official SDKs, OTLP exporters, HTTP/runtime instrumentation and slog bridge |
| [YAML organization v3](https://github.com/yaml/go-yaml) | YAML organization-maintained implementation; v3 stable API/security fixes |
| [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) | Official Go terminal detection; uses the already selected v0.47.0 and x/sys version |
| golang.org/x/time | Go project rate limiter for bounded JWKS refreshes |
| google.golang.org/protobuf and OTLP protocol | Official protocol implementations; local receiver verification |

Replaced github.com/MicahParks/keyfunc/v3 (and its jwkset dependency) with CoreOS verification.
Moved direct YAML usage from the archived gopkg.in/yaml.v3 repository to go.yaml.in/yaml/v3.
Signature parsing/verification remains inside established libraries; no home-grown JWT/JWK codec.

## Indirect dependencies

Official SDKs still bring community-maintained implementation dependencies. We do not claim
that every indirect helper is an official vendor package or independently popular. They are
inventoried and reviewed through their upstream provenance; do not import them directly merely
because they already appear in go.sum.

Examples in the runtime graph: go-jose via CoreOS, pgpassfile/pgservicefile/puddle via pgx,
LaunchDarkly's event/cache/evaluation modules plus established cache/JSON libraries, and
Google gRPC/Go x libraries/backoff/logr via OpenTelemetry. Small helpers such as josharian/intern
arrive through the SDK JSON stack, rather than a direct application choice. The service binary imports 67 external modules after the Gin migration. The full selected
module graph has 190 modules, including upstream optional adapters/test/build-only requirements,
and is recorded in dependencies.lock.json. Archived YAML imports and the removed JWKS wrapper
are absent from the service import graph; old YAML may remain in upstream test-only metadata. Use
`go mod why -m <module>` and `go mod graph` to inspect the introducing dependency.

## Development tools

| Tool | Upstream | Pinned version |
|---|---|---|
| Air | github.com/air-verse/air (established upstream; ~24k stars at review) | v1.67.4 |
| GolangCI-Lint | github.com/golangci/golangci-lint/v2 | v2.15.0 |
| govulncheck | golang.org/x/vuln (Go project) | v1.9.0 |
| Task (optional) | github.com/go-task/task/v3 | v3.54.0 |

Tools run at pinned versions outside the service module graph. Air is built only into the dev
image. Tool transitives are supplied by these upstream tool distributions; they are not runtime
service dependencies. Dockerfile/Compose use upstream Go, distroless, PostgreSQL and OpenTelemetry
images. Resolve image digests in deployment release automation when immutable image provenance
is required; a version tag is not a cryptographic provenance guarantee.

## Change workflow and enforcement

1. Review ownership, maintained upstream, usage, license, releases and security advisories.
2. Prefer a direct official/established dependency over a smaller wrapper, and record why.
3. Update modules with exact versions, run go mod tidy, and inspect both direct and indirect diffs.
4. Review all new/changed selected modules. Run `make deps-inventory` only after that review.
5. Commit the inventory diff with the rationale in this guide; run `make check`, `make test-race`,
   applicable integration/e2e tests and `make vuln`.

`make deps-check` compares the entire selected module graph/version set with the reviewed
inventory and rejects module replacements. It is part of make check/CI. It detects drift; it
does not automatically prove trust or approve a library. Do not regenerate the inventory just
to bypass a failure. Tool version changes also require review and consistent Air versions in
Makefile, Dockerfile and compose.dev.yaml. Provider/vendor credentials are never required by CI.

## Gin migration review — 2026-10-31

Gin v1.12.0 is pinned to the official upstream release, requires Go 1.25, and uses Huma v2.40.0's
shipped humagin adapter. Huma already selected this Gin version in its optional adapter module graph;
the change makes it an actual direct runtime dependency. No framework fork or wrapper is introduced.
See the [upstream release](https://github.com/gin-gonic/gin/releases/tag/v1.12.0) and
[release dependency manifest](https://github.com/gin-gonic/gin/blob/v1.12.0/go.mod).

Reviewed go.mod/go.sum, go mod graph, upstream manifests and the service import graph before updating
the inventory. The selected graph adds ten modules and updates testify's objx from v0.1.0 to v0.5.2.
Existing selected runtime versions remain unchanged; go.mod now records the Gin-required indirects.

| Newly selected / updated module | Introducing upstream and scope |
|---|---|
| go-playground/assert/v2 v2.2.0 | validator test assertions; not imported by service |
| google/gofuzz v1.0.0 | json-iterator tests; not imported by service |
| jordanlewis/gcassert pinned revision | quic-go development/testing; not imported by service |
| quic-go/go-ossfuzz-seeds v0.1.0 | quic-go fuzz corpus; not imported by service |
| xdg-go/pbkdf2 v1.0.0, scram v1.2.0, stringprep v1.0.4 | Official MongoDB driver authentication dependencies; those packages are not imported by service |
| youmark/pkcs8 pinned revision | Official MongoDB driver encryption dependency; not imported by service |
| go.uber.org/mock v0.6.0 | Gin testing; not imported by service |
| rsc.io/pdf v0.1.1 | Go x/arch support metadata; not imported by service |
| stretchr/objx v0.5.2 | Testify mock support; not imported by service |

The service import graph grows from 52 to 67 external modules through Gin: router/SSE, validator and
its translation/mimetype/URN helpers, terminal detection, TOML/YAML/MessagePack/BSON serialization,
QUIC/qpack and Go x/crypto implementation code. These come from Gin's established upstream dependency
stack; no application imports their helpers directly. The graph also retains Gin's alternative JSON
codecs and their implementation dependencies even when the default binary does not import them.

MongoDB is present for Gin's BSON serializer, not a configured database client; no MongoDB connection
or QUIC listener starts. PostgreSQL remains the reference durable users adapter, and optional providers
remain opt-in. The larger framework dependency surface is a tradeoff of selecting Gin, not a claim that
every transitive helper is independently popular. Continue graph review and vulnerability scanning on
updates. The inventory now records 190 modules; checksum verification and the quality gate remain required.

## Console logging dependency review — 2026-11-01

Promoted official Go `golang.org/x/term` v0.47.0 to a direct import solely for `IsTerminal`.
It was already selected in the reviewed graph, and its x/sys dependency is already selected.
No new module or selected version is required; the 190-module inventory remains unchanged.
Console rendering uses the standard library's slog attribute formatting, with no console wrapper
library or additional logging framework. The existing runtime graph adds the official term module.
