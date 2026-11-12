# Visper Web

Terminal-style web client for [Visper](https://github.com/HilthonTT/Visper), the anonymous, encrypted real-time chat app. You type shell-like commands (`help`, `setdevicecode`, `setjoincode`, `setroomkey`, `joinroom`, `themes`, ...) to send a room invite to your own Visper CLI through the Go API. The room key is sealed to your CLI's device key (X25519 + AES-GCM, via WebCrypto) before it is sent, so the API relays it without being able to read it.

Built with React 19, TanStack Start/Router, Vite and styled-components. Every colour comes from the active theme (`src/lib/themes.ts`).

## Screenshots

The layout follows the CLI's TUI: a header with the logo, page tabs and API status, the page in the middle, the companion on the right (wide screens, toggle with `alt+w`), and a footer with key hints.

![Terminal page with the companion panel](../../docs/assets/screenshots/web-terminal.png)

| `help`: every command is clickable | Join room: send an invite to your CLI |
| --- | --- |
| ![help output](../../docs/assets/screenshots/web-help.png) | ![Join room page](../../docs/assets/screenshots/web-join.png) |

| Settings: companion and theme | FAQ |
| --- | --- |
| ![Settings page](../../docs/assets/screenshots/web-settings.png) | ![FAQ page](../../docs/assets/screenshots/web-faq.png) |

| Light theme | Mobile |
| --- | --- |
| ![Light theme](../../docs/assets/screenshots/web-light.png) | <img src="../../docs/assets/screenshots/web-mobile.png" alt="Mobile layout" width="260"> |

Screenshots are taken at 2× with Playwright against `pnpm dev`. Retake them after visual changes.

## Pages and shortcuts

| Page | Key | What it does |
| --- | --- | --- |
| Terminal | `t` / `alt+1` | Shell-style commands (`help` lists them). Scrollback survives tab switches. |
| Join room | `j` / `alt+2` | Form version of `setdevicecode` + `setjoincode` + `setsecurecode` + `setroomkey` + `joinroom`. Opens first when the URL carries `?joinCode=`. |
| FAQ | `f` / `alt+3` | Same questions as the CLI's FAQ page. |
| Settings | `s` / `alt+4` | Pick the companion and the colour theme. Both are saved in the browser. |

Letter keys work when no text field has focus; `alt+1`–`alt+4` work anywhere. `alt+w` shows or hides the companion.

The companion images and the FAQ text are copies of the CLI's `internal/tui/embeds` files (`public/companions/`, `src/lib/companions.ts`, `src/lib/faq.ts`). Update both when one changes.

## Setup

Requires Node.js 22+ and pnpm (the version is pinned in `package.json` under `packageManager`; run `corepack enable` to use it).

```bash
pnpm install
cp .env.example .env   # then edit the values
pnpm dev               # http://localhost:3000
```

Room credentials can be prefilled from an invite link: `/?joinCode=...&secureCode=...#key=...`. The room key sits in the fragment, which browsers never send to a server; the page reads it, keeps it in memory only and removes it from the address bar. The device code comes from the CLI's settings page (press `c` there to copy it).

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
