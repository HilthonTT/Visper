# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.26-alpine AS builder
WORKDIR /src

ENV GOWORK=off CGO_ENABLED=0 GOOS=linux

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/proxy ./cmd/proxy

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/proxy /proxy

ENV PROXY_LISTEN_ADDR=:5004
EXPOSE 5004
USER nonroot:nonroot
ENTRYPOINT ["/proxy"]
