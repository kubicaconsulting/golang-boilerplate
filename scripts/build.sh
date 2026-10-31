#!/bin/sh
set -eu
GO=${GO:-go}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || printf dev)}
COMMIT=${COMMIT:-$(git rev-parse --verify HEAD 2>/dev/null || printf unknown)}
BUILD_DATE=${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
if [ -z "${DIRTY:-}" ]; then
  if [ "$COMMIT" = unknown ] || [ -n "$(git status --porcelain 2>/dev/null)" ]; then DIRTY=true; else DIRTY=false; fi
fi
# Safe linker tokens; metadata is data, never evaluated as shell commands.
for token in "$VERSION" "$COMMIT" "$BUILD_DATE" "$DIRTY"; do
  case "$token" in ''|*[!a-zA-Z0-9._+:-]*) printf '%s\n' 'Invalid build metadata token' >&2; exit 1;; esac
done
case "$DIRTY" in true|false) ;; *) printf '%s\n' 'DIRTY must be true or false' >&2; exit 1;; esac
metadata=boilerplate/internal/buildinfo
mkdir -p bin
CGO_ENABLED=0 "$GO" build -trimpath -ldflags="-s -w -X $metadata.version=$VERSION -X $metadata.commit=$COMMIT -X $metadata.builtAt=$BUILD_DATE -X $metadata.dirty=$DIRTY" -o bin/service ./cmd/service
