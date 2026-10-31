#!/bin/sh
set -eu
: "${VERSION:?Set VERSION to a unique release identifier}"
IMAGE_NAME=${IMAGE_NAME:-microservice}
case "$VERSION" in ''|dev|*[!a-zA-Z0-9._-]*) printf '%s\n' 'Use a unique Docker-tag-safe VERSION, not dev' >&2; exit 1;; esac
COMMIT=$(git rev-parse --verify HEAD) || { printf '%s\n' 'Commit the codebase before creating a release' >&2; exit 1; }
if [ -n "$(git status --porcelain)" ]; then printf '%s\n' 'Release images require a clean committed working tree' >&2; exit 1; fi
if docker image inspect "$IMAGE_NAME:$VERSION" >/dev/null 2>&1; then printf '%s\n' 'Release tag already exists locally; use a new VERSION' >&2; exit 1; fi
BUILD_DATE=${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
docker build --target production --build-arg "VERSION=$VERSION" --build-arg "COMMIT=$COMMIT" --build-arg "BUILD_DATE=$BUILD_DATE" --build-arg DIRTY=false -t "$IMAGE_NAME:$VERSION" .
# Record the immutable local image ID. Registries assign their own manifest digest on push.
docker image inspect "$IMAGE_NAME:$VERSION" --format '{{.Id}}'
