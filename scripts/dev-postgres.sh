#!/bin/sh
set -eu
# Optional parity workflow; ordinary make dev remains infrastructure-free.
docker compose --profile database up -d --wait postgres
export APP_DATABASE_DRIVER=postgres
export APP_DATABASE_DSN="${APP_DATABASE_DSN:-postgres://app:local-only-password@localhost:${POSTGRES_PORT:-5432}/app?sslmode=disable}"
"${GO:-go}" run ./cmd/service --migrate
exec make dev
