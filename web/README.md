# Visper Web

Terminal-style web client for [Visper](https://github.com/HilthonTT/Visper), the anonymous, encrypted real-time chat app. You type shell-like commands (`help`, `setuserid`, `setjoincode`, `joinroom`, `themes`, ...) to send a room invite to your own Visper CLI sessions through the Go API.

Built with React 19, TanStack Start/Router, Vite, styled-components and Tailwind CSS.

## Setup

Requires Node.js 22+ and pnpm (the version is pinned in `package.json` under `packageManager`; run `corepack enable` to use it).

```bash
pnpm install
cp .env.example .env   # then edit the values
pnpm dev               # http://localhost:3000
```

Room credentials can be prefilled from the URL: `/?joinCode=...&secureCode=...`.

## Environment variables

| Variable               | Description                                                       |
| ---------------------- | ----------------------------------------------------------------- |
| `VITE_BACKEND_API_URL` | Base URL of the Visper API, no trailing slash (e.g. `http://localhost:5005`). |

Vite inlines `VITE_*` variables at build time, so set them before running `pnpm build`.

## Scripts

| Script           | What it does                                      |
| ---------------- | ------------------------------------------------- |
| `pnpm dev`       | Start the dev server on port 3000                 |
| `pnpm build`     | Production build (output in `.output/`)           |
| `pnpm preview`   | Serve the production build locally                |
| `pnpm lint`      | Lint and check formatting with Biome              |
| `pnpm format`    | Format the code with Biome                        |
| `pnpm typecheck` | Type-check with `tsc --noEmit`                    |
| `pnpm test`      | Run the unit tests with Vitest                    |

You can also run the production server directly with `node .output/server/index.mjs`.

## Layout

```
src/
  routes/       TanStack file routes (__root.tsx holds the <head>)
  components/   terminal UI, one component per command in commands/
  contexts/     user (ids/codes) and per-history-entry terminal context
  hooks/        theme, URL params, env variables
  lib/          pure helpers (command parsing, completion, API client, themes)
```
