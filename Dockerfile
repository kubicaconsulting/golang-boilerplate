# syntax=docker/dockerfile:1
FROM golang:1.27.3-bookworm AS base
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Development tooling stays outside the production image/module graph.
FROM base AS dev
ARG AIR_VERSION=v1.67.4
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/air-verse/air@${AIR_VERSION}
COPY . .
ENV APP_ENV=local APP_HTTP_ADDRESS=:8080
EXPOSE 8080
ENTRYPOINT ["air", "-c", ".air.toml"]

FROM base AS build
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG DIRTY=true
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    VERSION="$VERSION" COMMIT="$COMMIT" BUILD_DATE="$BUILD_DATE" DIRTY="$DIRTY" ./scripts/build.sh && cp bin/service /service

FROM gcr.io/distroless/static-debian12:nonroot AS production
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.created="$BUILD_DATE"
WORKDIR /app
COPY --from=build /service /app/service
COPY --from=build /src/configs /app/configs
ENV APP_ENV=production
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/service"]
