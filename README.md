<div align="center">

<img src=".github/assets/logo.svg" alt="Visper logo" width="112" height="112">

# Visper

**Anonymous, end-to-end encrypted chat rooms — right from your terminal.**

[![CI](https://github.com/HilthonTT/Visper/actions/workflows/ci.yml/badge.svg)](https://github.com/HilthonTT/Visper/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](go.work)
[![License: MIT](https://img.shields.io/badge/license-MIT-10B981)](LICENSE)

</div>

---

Visper lets you spin up a throwaway chat room, share a join code (or a QR
code), and talk without accounts. Messages are encrypted on the client before
they ever reach the server. It ships as a polished TUI built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea), with a terminal-style
web client and an optional local-LLM "enhance my message" feature.

## Features

- **Ephemeral rooms**: create a room, share the join code or QR invite, and it expires on its own
- **End-to-end encryption**: room keys stay on clients; the server stores ciphertext
- **Real-time**: WebSocket messaging, edits, deletes, member join/leave and kicks
- **File & image sharing**: with inline image previews in the terminal
- **AI message enhancement**: rewrite messages with a local [Ollama](https://ollama.com) model, no data leaves your machine
- **Production-minded backend**: rate limiting, load-balancing proxy with health checks, Redis Sentinel, OpenTelemetry tracing, Prometheus metrics

## Architecture

```
   ┌───────────┐               ┌───────────┐
   │    cli    │               │    web    │
   │   (TUI)   │               │  (React)  │
   └─┬───────┬─┘               └─────┬─────┘
     │       │ api-sdk               │
     │       ▼                       ▼
     │  ┌─────────────────────────────────┐
     │  │ proxy  (load balancer,          │
     │  │ throttling, metrics)  optional  │
     │  └────────────────┬────────────────┘
     │                   ▼
     │  ┌─────────────────────────────────┐
     │  │ api  (Gin, REST + WebSockets)   │
     │  └───────┬─────────────────┬───────┘
     │          ▼                 ▼
     │      Postgres     Redis (master/replica + Sentinel)
     │                            ▲
     ▼  "enhance message"         │ user lookup, cache, queue
   ┌─────────────────┐            │
   │ ai  (FastAPI +  ├────────────┘
   │ arq worker)     ├──► Ollama
   └─────────────────┘
```

| Directory            | What it is                                                         | Stack                                  |
| -------------------- | ------------------------------------------------------------------ | -------------------------------------- |
| [`api/`](api)        | REST + WebSocket backend (clean architecture)                      | Go, Gin, GORM, Redis, OpenTelemetry    |
| [`api-sdk/`](api-sdk) | Typed Go client for the API, used by the CLI                     | Go                                     |
| [`cli/`](cli)        | The `visper` terminal client                                       | Go, Bubble Tea v2, Lip Gloss v2        |
| [`proxy/`](proxy)    | Reverse proxy / load balancer with hierarchical throttling         | Go, Prometheus                         |
| [`ai/`](ai)          | Message-enhancement service and background worker                  | Python, FastAPI, arq, Ollama           |
| [`web/`](web)        | Terminal-style web client                                          | React 19, TanStack Start, Vite         |

The Go modules are tied together with a [Go workspace](go.work), so the CLI
always builds against the local `api-sdk`.

## Getting started

### Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) with Compose
- Optional: Python 3.12+ (AI service), Node.js 22+ and pnpm (web client),
  [air](https://github.com/air-verse/air) (live reload), [golangci-lint](https://golangci-lint.run/) v2

### Run everything in Docker

```bash
git clone https://github.com/HilthonTT/Visper.git
cd Visper
cp .env.example .env        # dev defaults: change for anything public
make up-app                 # = docker compose --profile app up -d --build
```

This starts Postgres, Redis (master, replica, 3 Sentinels), Jaeger, Ollama
(pulling `qwen2.5:3b` on first run), the API on `:5005`, the proxy on `:5004`
and the AI service on `:8088`.

> Have an NVIDIA GPU? Add `-f docker-compose.gpu.yml` to run Ollama on it.

Then start the client:

```bash
make cli                    # or: cd cli && go run ./cmd/visper
```

### Local development

Run the infrastructure in Docker and the services on your machine:

```bash
make up                     # Postgres, Redis, Jaeger, Ollama
make dev-api                # API with live reload
make dev-proxy              # proxy with live reload (optional)
make dev-ai                 # AI service (creates ai/.venv on first run)
make dev-web                # web client on http://localhost:3000
make cli                    # the TUI
```

The API reads `api/infrastructure/config/config-<APP_ENV>.yml`
(`development` by default; this file is git-ignored, so copy
`config-docker.yml` to start). Every key can be overridden with a
`VISPER_<SECTION>_<KEY>` environment variable; see
[`api/.env.example`](api/.env.example).

### Configuration

| Component | Reference                                    |
| --------- | -------------------------------------------- |
| Compose   | [`.env.example`](.env.example)               |
| API       | [`api/.env.example`](api/.env.example)       |
| Proxy     | [`proxy/.env.example`](proxy/.env.example)   |
| AI        | [`ai/.env.example`](ai/.env.example)         |
| Web       | [`web/.env.example`](web/.env.example)       |
| CLI       | `VISPER_BASE_URL` (default `http://localhost:5005`), `VISPER_AI_BASE_URL` (default `http://localhost:8088`) |

The CLI stores its settings in your OS config directory
(`visper/config.json`) and logs to your OS cache directory (`visper/visper.log`).

## Development

```bash
make help        # list all targets
make check       # everything CI runs: gofmt, vet, golangci-lint, tests, ruff, pytest, biome, tsc, vitest
make build       # build visper, visper-api and visper-proxy into ./bin
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow and conventions.

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## Credits

- [terminal.shop](https://www.terminal.shop/) for TUI inspiration
- [superfile](https://superfile.dev/) for file explorer ideas
- [rcdmk/go-ratelimiter](https://github.com/rcdmk/go-ratelimiter) for rate limiting
- [gofr-dev](https://github.com/gofr-dev) for the metrics design
- [support.tools](https://support.tools/)

## License

[MIT](LICENSE) © 2025 Hilthon
