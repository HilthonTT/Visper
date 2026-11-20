# Visper Web

Terminal-style web client for [Visper](https://github.com/HilthonTT/Visper), the anonymous, encrypted real-time chat app. The **Chat** page creates or joins a room right in the browser (see [Chat in the browser](#chat-in-the-browser)). On the Terminal and Join room pages you type shell-like commands (`help`, `setdevicecode`, `setjoincode`, `setroomkey`, `joinroom`, `themes`, ...) to send a room invite to your own Visper CLI through the Go API. The room key is sealed to your CLI's device key (X25519 + AES-GCM, via WebCrypto) before it is sent, so the API relays it without being able to read it.

Built with React 19, TanStack Start/Router, Vite and styled-components. Every colour comes from the active theme (`src/lib/themes.ts`).

## Demos

![Joining a room in the browser and chatting](../../docs/assets/demos/web-chat.gif)

More (reactions, replies, search, the terminal): [docs/demos](../../docs/demos/README.md#web-client).

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

**Verify keys:** click a member's name to compare key fingerprints with them, read out over a call or in person ([how it works](../../docs/security-model.md#verifying-keys)).

![Keys panel with a verified member](../../docs/assets/screenshots/web-keys.png)

Screenshots are taken at 2× with Playwright against the production build: `bash docs/demos/record-web.sh screenshots` ([docs/demos](../../docs/demos/README.md#web-demos)). Retake them after visual changes.

## Pages and shortcuts

| Page | Key | What it does |
| --- | --- | --- |
| Terminal | `t` / `alt+1` | Shell-style commands (`help` lists them). Scrollback survives tab switches. |
| Chat | `c` / `alt+2` | Create or join a room and chat in the browser. Opens first when the URL carries `?joinCode=`. Stays connected while other pages are shown. |
| Join room | `j` / `alt+3` | Send a room invite to your CLI instead: form version of `setdevicecode` + `setjoincode` + `setsecurecode` + `setroomkey` + `joinroom`. |
| FAQ | `f` / `alt+4` | Same questions as the CLI's FAQ page. |
| Settings | `s` / `alt+5` | Pick the companion and the colour theme. Both are saved in the browser. |

Letter keys work when no text field has focus; `alt+1`–`alt+5` work anywhere. `alt+w` shows or hides the companion.

The companion images and the FAQ text are copies of the CLI's `internal/tui/embeds` files (`public/companions/`, `src/lib/companions.ts`, `src/lib/faq.ts`). Update both when one changes.

## Setup

Requires Node.js 22+ and pnpm (the version is pinned in `package.json` under `packageManager`; run `corepack enable` to use it).

```bash
pnpm install
cp .env.example .env   # then edit the values
pnpm dev               # http://localhost:3000
```

Room credentials can be prefilled from an invite link: `/?joinCode=...&secureCode=...#key=...`. The room key sits in the fragment, which browsers never send to a server; the page reads it, keeps it in memory only and removes it from the address bar. The device code comes from the CLI's settings page (press `c` there to copy it).

## Chat in the browser

Join with an invite link (`/?joinCode=…&secureCode=…#key=…`, from a CLI's `ctrl+p`) or by pasting an invite code (`ABC123.<key>`). Or start a new room (it lives 1 hour, 24 hours or 7 days): the room key is generated in the page, and the **invite** button in the room bar shows the invite code and link to share, both carrying the key. Everything is end-to-end encrypted in the page with the room key (NaCl secretbox via [tweetnacl](https://github.com/dchest/tweetnacl-js), the same format as the Go SDK): messages, reactions and images. The key is kept in memory only, so reloading the page means joining again, unless you ask this browser to remember the room (see [Remembered rooms](#remembered-rooms)).

In a room:

- **Messages**: `enter` sends, `shift+enter` adds a line, `↑` in an empty box edits your last message, `esc` cancels a reply or edit. Hover a message (or tap, on touch screens) for reply, react, edit and delete.
- **Replies** quote the original message; click the quote to jump to it.
- **Reactions**: one per person per message. Clicking a reaction toggles yours.
- **Images**: `img` encrypts an image (up to 5 MB) before uploading it; image messages from the CLI are downloaded and decrypted in the page.
- **Presence and typing**: the member list shows who is connected (`members` on narrow screens), and a line under the messages shows who is typing.
- **Read receipts**: "seen by …" marks the last message each member has read. Yours is sent for the newest message while the chat is on screen.
- **Search** filters the loaded messages. It runs in the page: the server only has ciphertext, so it can't search.
- **Files** lists the room's uploaded files with their size, uploader and date. The server stores them as `file.enc`, so names come from the loaded messages that share them (click one to jump to it); older ones show as "encrypted file". You can delete your own files; the owner can delete any.
- **Owner tools**: the owner sees **kick** next to each member (on hover), **new code** (the old join code stops letting people in; the room key is unchanged) and **delete** (removes the room for everyone). Kicking someone rotates the room key, as when they leave.

### Remembered rooms

By default nothing survives a reload. Tick **remember the room on this browser** when joining or creating, or press **remember** in the room bar, and the page keeps the room's keys, your identity for it (so your key fingerprint stays the same) and the members you verified in IndexedDB. The Chat page then lists the room to reopen without an invite. Reopening checks that the API still knows this browser as the same member, and that the owner is the one pinned when you first entered.

The keys are stored **unencrypted**: anyone with this browser profile, and any script the site serves, can read them. Only remember rooms on a device you trust. **forget** in the room bar or in the list deletes them; leaving, being kicked, the room being deleted or expiring forgets the room too. The browser's API session lasts 30 days: after it expires, a remembered room can't be reopened.

The browser is identified by the API's HttpOnly `visper_user_id` cookie, which holds a private user token (a browser WebSocket can't send the `X-User-Token` header), so chat requests are sent with credentials. **The API must list this site in `VISPER_CORS_ALLOWORIGINS`** (e.g. `http://localhost:3000`): with `*`, browsers refuse to send credentials and the Chat page reports that it can't reach the API.

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
