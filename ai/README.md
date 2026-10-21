# visper-ai

FastAPI service that rewrites chat messages (style/tone) with a local Ollama model
(`qwen2.5:3b` by default), with a rule-based fallback, Redis caching and per-user rate
limiting. The Go API / `api-sdk` calls it at `POST /api/v1/ai/enhance` and
`GET /api/v1/ai/health` (base URL `VISPER_AI_BASE_URL`, default `http://localhost:8088`),
passing the user via the `X-User-ID` header; sessions are read from Redis keys
`visper:user:<id>` written by the Go API.

## Layout

```
src/visper_ai/
  main.py            # ASGI app (`visper_ai.main:app`)
  api/               # routers (/api/v1/health, /ready, /ai/*) + dependencies (auth, rate limit)
  core/              # config, logging, lifespan/setup, Ollama client, enhancement service, worker
  middleware/        # request logging, client cache headers
  schemas/           # pydantic models
tests/
```

## Setup

Requires Python 3.12+ (tested on 3.14), Redis and [Ollama](https://ollama.com)
(`ollama pull qwen2.5:3b`).

```bash
cd ai
python -m venv .venv
source .venv/bin/activate        # Windows: .venv\Scripts\activate
pip install -e ".[dev]"
cp .env.example .env             # .env is read from the current working directory
```

## Run the API

```bash
uvicorn visper_ai.main:app --host 0.0.0.0 --port 8088 --reload
# or: visper-ai   (uses APP_HOST / APP_PORT)
```

Docs are served at `/docs` unless `ENVIRONMENT=production`.

## Run the worker

```bash
arq visper_ai.core.worker.settings.WorkerSettings
# or: visper-ai-worker
```

## Test & lint

```bash
pytest
ruff check .
ruff format --check .
mypy            # optional
```

Tests fake Redis (`fakeredis`) and Ollama; no external services are needed.

## Docker

```bash
docker build -t visper-ai .
docker run --rm -p 8088:8088 --env-file .env \
  -e REDIS_CACHE_HOST=host.docker.internal -e OLLAMA_HOST=host.docker.internal visper-ai
# worker
docker run --rm --env-file .env visper-ai arq visper_ai.core.worker.settings.WorkerSettings
```

## Configuration

See `.env.example` for every variable. Logs go to stdout and, when `FILE_LOG_ENABLED=true`,
to `$LOG_DIR/app.log` (default `./logs`, relative to the working directory).
