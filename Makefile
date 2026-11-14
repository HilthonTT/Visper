SHELL := bash
.DEFAULT_GOAL := help

GO_MODULES := services/api services/proxy clients/cli sdk/go
BIN_DIR    := bin
EXE        := $(if $(filter Windows_NT,$(OS)),.exe,)

AI_VENV := services/ai/.venv
AI_BIN  := $(AI_VENV)/$(if $(filter Windows_NT,$(OS)),Scripts,bin)

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[32m%-14s\033[0m %s\n", $$1, $$2}'

# ── Build ───────────────────────────────────────────────────
.PHONY: build
build: ## Build the api, proxy and visper CLI binaries into ./bin
	cd services/api   && go build -o ../../$(BIN_DIR)/visper-api$(EXE)   ./cmd/api
	cd services/proxy && go build -o ../../$(BIN_DIR)/visper-proxy$(EXE) ./cmd/proxy
	cd clients/cli    && go build -o ../../$(BIN_DIR)/visper$(EXE)       ./cmd/visper

.PHONY: release-snapshot
release-snapshot: ## Build the CLI release archives into ./dist without publishing (needs goreleaser)
	goreleaser release --snapshot --clean --skip=publish

# ── Quality ─────────────────────────────────────────────────
.PHONY: check
check: go-check ai-check web-check ## Run every formatter check, linter and test suite

.PHONY: go-check
go-check: ## gofmt, vet, golangci-lint and tests for every Go module
	@for m in $(GO_MODULES); do \
		echo "── $$m"; \
		( cd $$m && test -z "$$(gofmt -l .)" && go vet ./... \
			&& golangci-lint run --config $(CURDIR)/.golangci.yml ./... \
			&& go test -race ./... ) || exit 1; \
	done

.PHONY: fmt
fmt: ## Format Go, Python and web sources
	@for m in $(GO_MODULES); do gofmt -w $$m; done
	$(AI_BIN)/ruff format services/ai
	cd clients/web && pnpm format

.PHONY: tidy
tidy: ## go mod tidy every Go module
	@for m in $(GO_MODULES); do (cd $$m && go mod tidy); done

$(AI_VENV):
	python -m venv $(AI_VENV)
	$(AI_BIN)/python -m pip install -e "./services/ai[dev]"

.PHONY: ai-check
ai-check: $(AI_VENV) ## Lint and test the Python AI service
	cd services/ai && .venv/$(notdir $(AI_BIN))/ruff check . && .venv/$(notdir $(AI_BIN))/ruff format --check . && .venv/$(notdir $(AI_BIN))/pytest

.PHONY: web-check
web-check: ## Lint, typecheck, test and build the web client
	cd clients/web && pnpm install --frozen-lockfile && pnpm lint && pnpm typecheck && pnpm test && pnpm build

# ── Run ─────────────────────────────────────────────────────
.PHONY: up
up: ## Start infrastructure (Postgres, Redis, Jaeger, Ollama)
	docker compose up -d

.PHONY: up-app
up-app: ## Start infrastructure and all Visper services in Docker
	docker compose --profile app up -d --build

.PHONY: down
down: ## Stop everything started with docker compose
	docker compose --profile app down

.PHONY: dev-api
dev-api: ## Run the API with live reload (air)
	cd services/api && air

.PHONY: dev-proxy
dev-proxy: ## Run the proxy with live reload (air)
	cd services/proxy && air

.PHONY: dev-ai
dev-ai: $(AI_VENV) ## Run the AI service API
	cd services/ai && .venv/$(notdir $(AI_BIN))/uvicorn visper_ai.main:app --reload --port 8088

.PHONY: dev-web
dev-web: ## Run the web client dev server
	cd clients/web && pnpm dev

.PHONY: cli
cli: ## Run the visper TUI
	cd clients/cli && go run ./cmd/visper

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN_DIR) services/api/tmp services/proxy/tmp clients/web/dist
