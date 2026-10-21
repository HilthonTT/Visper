# Contributing to Visper

Thanks for your interest in improving Visper! This guide covers how to get set
up and what a good pull request looks like.

## Getting set up

1. Fork and clone the repository.
2. Install the prerequisites listed in the [README](README.md#prerequisites).
3. Start the infrastructure with `make up`.
4. Run `make check` once to make sure everything passes before you change anything.

## Repository layout

This is a monorepo. Each top-level directory is an independent component with
its own dependencies; see the [architecture table](README.md#architecture).
The Go modules share a committed `go.work`, so changes to `api-sdk` are picked
up by `cli` immediately.

Go code follows the standard layout: binaries under `cmd/<name>`, private
packages under `internal/`.

## Making changes

- **Branch** from `main` and keep each pull request focused on one change.
- **Commits** use [Conventional Commits](https://www.conventionalcommits.org/):
  `feat(cli): ...`, `fix(api): ...`, `chore(deps): ...`, `docs: ...`.
- **Tests**: add or update tests for behaviour you change. Table-driven tests
  with the standard `testing` package for Go, `pytest` for `ai/`, `vitest`
  for `web/`.
- **API contract**: the CLI (via `api-sdk`), the web client and the proxy all
  talk to the API. If you change a route, JSON field or WebSocket event, update
  every consumer in the same pull request and call it out in the description.
- **Config**: new settings need a sensible default and an entry in the
  relevant `.env.example`.
- **Secrets**: never commit real credentials, tokens, room codes or `.env`
  files.

## Before opening a pull request

```bash
make fmt     # format Go, Python and web code
make check   # what CI runs
```

`make check` runs, per component:

| Component | Checks                                                     |
| --------- | ---------------------------------------------------------- |
| Go        | `gofmt`, `go vet`, `golangci-lint`, `go test -race`        |
| `ai/`     | `ruff check`, `ruff format --check`, `pytest`              |
| `web/`    | `biome check`, `tsc --noEmit`, `vitest`, `vite build`      |

## Updating dependencies

Dependabot opens weekly grouped updates. To update by hand:

```bash
cd <module> && go get -u ./... && go mod tidy   # Go
cd web && pnpm up --latest                     # web
```

Bump the `go` directive in every `go.mod` and in `go.work` together.

## Reporting bugs and requesting features

Use the issue templates. For security issues, follow [SECURITY.md](SECURITY.md)
instead of opening a public issue.

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
