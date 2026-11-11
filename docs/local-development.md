# Running Visper locally

Infrastructure (Postgres, Redis, Jaeger, Ollama) runs in Docker. The services
run either in Docker as well (quickest) or directly on your machine (for
development with live reload).

## Prerequisites

| Tool | Version | Needed for |
| ---- | ------- | ---------- |
| [Docker](https://docs.docker.com/get-docker/) + Compose | recent | infrastructure, always |
| [Go](https://go.dev/dl/) | 1.27+ | API, proxy, CLI |
| [Python](https://www.python.org/downloads/) | 3.12+ | AI service |
| [Node.js](https://nodejs.org/) + [pnpm](https://pnpm.io/installation) | Node 22+ (24 recommended), pnpm 10 | web client |
| GNU `make` + bash | any | the `make` shortcuts (optional) |
| [air](https://github.com/air-verse/air) | latest | live reload (optional): `go install github.com/air-verse/air@latest` |
| [golangci-lint](https://golangci-lint.run/) | v2 | `make check` (optional) |

> **Windows:** run `make` from Git Bash (`choco install make` if it's missing).
> Every `make` target below also shows the plain command it runs, so you can
> skip `make` entirely and use PowerShell.

```bash
git clone https://github.com/HilthonTT/Visper.git
cd Visper
cp .env.example .env    # dev defaults; change them for anything public
```

## Option A: everything in Docker

```bash
make up-app             # docker compose --profile app up -d --build
make cli                # cd clients/cli && go run ./cmd/visper
```

The first start pulls the `qwen2.5:3b` model into Ollama (~2 GB), so AI
enhancement only works once `docker logs visper-ollama-init` reports success.
NVIDIA GPU? Add `-f docker-compose.yml -f docker-compose.gpu.yml` to the compose command.

Stop with `make down` (`docker compose --profile app down`). Add `-v` to also
wipe the database and Redis volumes.

## Option B: services on your machine

### 1. Start the infrastructure

```bash
make up                 # docker compose up -d
```

### 2. API (port 5005)

```bash
make dev-api            # cd services/api && air
# without air:
cd services/api && go run ./cmd/api
```

The API loads `services/api/infrastructure/config/config-development.yml`,
which already points at the Docker infrastructure on `localhost`. Override any
key with an environment variable named `VISPER_<SECTION>_<KEY>`, for example
`VISPER_POSTGRES_PASSWORD=...`. Database migrations run on startup.

Check it: `curl http://localhost:5005/health`

### 3. Proxy (port 5004, optional)

The CLI and web client talk to the API directly by default. To put the
load-balancing proxy in front:

```bash
make dev-proxy          # cd services/proxy && air
# without air:
cd services/proxy && go run ./cmd/proxy
```

It forwards to `http://localhost:5005` by default. See
[`services/proxy/.env.example`](../services/proxy/.env.example) for throttling
and Redis Sentinel settings. Point clients at it with
`VISPER_BASE_URL=http://localhost:5004`.

The API rate-limits per client IP and ignores `X-Forwarded-For` unless the
sender is a trusted proxy. With the proxy in front, start the API with
`VISPER_SERVER_TRUSTEDPROXIES=127.0.0.1`; otherwise every client shares the
proxy's limit. (Docker Compose already sets this for the `proxy` container.)

### 4. AI service (port 8088, optional)

```bash
cp services/ai/.env.example services/ai/.env
make dev-ai             # creates services/ai/.venv on first run, then starts uvicorn
```

Without `make`:

```bash
cd services/ai
python -m venv .venv
.venv/bin/pip install -e ".[dev]"          # Windows: .venv\Scripts\pip
.venv/bin/uvicorn visper_ai.main:app --reload --port 8088
```

Background jobs (`/ai/enhance/async`, `/ai/enhance/batch`) also need the worker:

```bash
cd services/ai && .venv/bin/arq visper_ai.core.worker.settings.WorkerSettings
```

API docs: <http://localhost:8088/docs>

### 5. Clients

**Terminal client:**

```bash
make cli                # cd clients/cli && go run ./cmd/visper
```

| Variable | Default |
| -------- | ------- |
| `VISPER_BASE_URL` | `http://localhost:5005` |
| `VISPER_AI_BASE_URL` | `http://localhost:8088` |

Settings live in your OS config directory (`visper/config.json`) and logs in your
OS cache directory (`visper/visper.log`).

**Web client** (<http://localhost:3000>):

```bash
cp clients/web/.env.example clients/web/.env
cd clients/web && pnpm install && cd ../..
make dev-web            # cd clients/web && pnpm dev
```

## Ports at a glance

| Service | Port | | Service | Port |
| ------- | ---- | - | ------- | ---- |
| API | 5005 | | Postgres | 5432 |
| Proxy | 5004 | | Redis master / replica | 6379 / 6380 |
| AI service | 8088 | | Redis Sentinels | 36379-36381 |
| Web client | 3000 | | Jaeger UI | 16686 |
| Ollama | 11434 | | OTLP (HTTP / gRPC) | 4318 / 4317 |

## Checks and builds

```bash
make check      # gofmt, vet, golangci-lint, go test -race, ruff, pytest, biome, tsc, vitest, vite build
make build      # bin/visper, bin/visper-api, bin/visper-proxy
make help       # every target
```

`go test -race` needs cgo (a C compiler). On Windows without gcc, run
`go test ./...` instead.

## Troubleshooting

| Symptom | Fix |
| ------- | --- |
| `bind: address already in use` / port taken | Stop the other process, or change the port mapping in `docker-compose.yml`. A local Postgres or Redis service is the usual suspect. |
| API: `config file not found` | Run it from `services/api` (or a subdirectory), or set `APP_ENV` to match an existing `config-<env>.yml`. |
| API can't reach Postgres or Redis | `docker compose ps`: wait until `postgres` and `redis-master` are `healthy`. |
| AI service: `NOAUTH` / auth errors | The Redis passwords in `services/ai/.env` must match `REDIS_PASSWORD` in the root `.env`. |
| AI enhancement returns the rule-based fallback | The Ollama model is still downloading: `docker logs -f visper-ollama-init`. |
| `make` not found on Windows | Use Git Bash with `choco install make`, or run the commands shown above directly. |
