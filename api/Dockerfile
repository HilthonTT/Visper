# syntax=docker/dockerfile:1

# Build context: the api/ directory. The api module does not import any other
# module of the go.work workspace, so it is built standalone with GOWORK=off.
#
#   docker build -t visper-api ./api

FROM golang:1.26-alpine AS builder

WORKDIR /src

ENV CGO_ENABLED=0 \
    GOWORK=off

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/visper-api ./cmd/api

# Writable runtime directories, owned by the distroless "nonroot" user (65532).
RUN mkdir -p /out/app/data /out/app/uploads /out/app/logs


FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/visper-api /app/visper-api
# The binary looks for config-<APP_ENV>.yml under ./infrastructure/config.
COPY --chown=nonroot:nonroot infrastructure/config/config-docker.yml /app/infrastructure/config/config-docker.yml
COPY --from=builder --chown=nonroot:nonroot /out/app/ /app/

# Settings can be overridden with VISPER_* variables, e.g.
# VISPER_POSTGRES_PASSWORD, VISPER_REDIS_PASSWORD, VISPER_SENTRY_DSN.
ENV APP_ENV=docker \
    GIN_MODE=release

# Broker log, consumer offsets and uploaded files.
VOLUME ["/app/data", "/app/uploads"]

EXPOSE 5005

USER nonroot:nonroot

ENTRYPOINT ["/app/visper-api"]
