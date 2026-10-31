# Build, release and run

A Git repository is initialized, with no generated commit or remote. Review the scaffold and make
your initial commit before creating releases. No registry, cloud provider or deployment system is
assumed. Rename the boilerplate module and linker package path in scripts/build.sh
before adopting the template under another module name.

make build embeds version, commit, UTC build time and dirty status in bin/service. Overrides are
VERSION, COMMIT, BUILD_DATE and DIRTY. Local/uncommitted builds identify themselves as dev/unknown
and dirty. bin/service --version prints JSON without loading configuration, opening ports or
requiring credentials. Startup logs the same metadata; telemetry service.version uses the version.
make version reports a development go run build; use the actual built executable to inspect releases.

```sh
make build
./bin/service --version
# After review and a clean Git commit:
VERSION=v1.0.0 IMAGE_NAME=my-service make release-image
```

release-image requires a unique non-dev Docker-safe version and a clean committed tree. It refuses
to overwrite a locally existing tag, passes metadata into the binary and OCI labels, and prints the
local immutable image ID. It does not publish anything. Registry tag protection must be configured
in your chosen registry; local checks cannot enforce remote immutability. After publication, promote
the same manifest digest across environments. Inject configuration/secrets at run time, rather than
rebuilding for each environment. A release combines an immutable build with deployment configuration.

On pushes, CI creates a version containing commit SHA, run ID and attempt after checks, integration
and container verification succeed. It uploads the Docker image archive, binary metadata, image
inspection and SHA256SUMS as a uniquely named GitHub Actions artifact retained for 14 days. Download,
verify checksums and docker load the archive before publishing it through your chosen release system.
GitHub artifacts are a transport foundation, not a permanent image registry or deployment workflow.
PRs run checks without producing release artifacts. No production credentials are required.

Run migrations as an explicit administrative process from the same image and deployment environment.
The migration budget is independent of shutdown. Avoid release changes that require incompatible
schema updates while old replicas run; use expand/contract migrations.

See the [12-factor build/release/run guidance](https://12factor.net/build-release-run) and
[GitHub artifact documentation](https://docs.github.com/en/actions/tutorials/store-and-share-data).
