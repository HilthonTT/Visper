# Demos

Every GIF here is recorded from the real app against a local stack, using
[VHS](https://github.com/charmbracelet/vhs). The `.tape` files in this folder are
the scripts.

## Chat in an encrypted room

Join with an invite code and talk. Messages are encrypted on the client with a
key carried by the invite code, so the server only stores ciphertext.

![Joining a room and chatting](../assets/demos/chat.gif)

## Create a room and invite people

`n` then `Enter` creates a room. `Ctrl+P` shows a QR invite that carries the
join code and secure token.

![Creating a room and showing its QR invite](../assets/demos/create-room.gif)

## Edit and delete your messages

`Ctrl+E` enters edit mode. `↑/↓` select one of your messages, `Enter` edits it
and `Ctrl+D` deletes it.

![Editing and deleting messages](../assets/demos/edit-delete.gif)

## Share images

`Ctrl+U` opens a file picker. Images preview inline in the terminal.

![Uploading an image that previews inline](../assets/demos/file-share.gif)

## Rewrite a message with local AI

`Ctrl+A` rewrites your draft with a local [Ollama](https://ollama.com) model. The
text never leaves your machine.

![Enhancing a draft message with AI](../assets/demos/ai-enhance.gif)

## Menu, FAQ, settings and help

`m` opens the menu palette and `tab` or `←/→` move between the tabs. Settings
pick the companion shown beside the chat, and `?` lists every shortcut.

![Menu palette, FAQ, settings and help](../assets/demos/tour.gif)

## Web client

The same features in the browser ([clients/web](../../clients/web)), recorded with
[Playwright](https://playwright.dev) against the production build.

### Chat in the browser

Paste an invite code on the **Chat** tab and talk. The page encrypts and
decrypts with the room key itself; the server only relays ciphertext.

![Joining a room in the browser and chatting](../assets/demos/web-chat.gif)

### React, reply and search

Hover a message to react or reply. Search runs in the page over the loaded
messages, since the server can't read them.

![Reacting, replying and searching in the web chat](../assets/demos/web-reactions.gif)

### Terminal, themes and pages

![The web terminal, a theme change and the other pages](../assets/demos/web-tour.gif)

## Re-recording

You need Docker and Go. The script runs VHS in a container, so you don't need
to install it.

```bash
docker compose --profile app up -d --build   # full stack; first run pulls the Ollama model
bash docs/demos/record.sh                    # every tape -> docs/assets/demos/*.gif
bash docs/demos/record.sh chat file-share    # just these
```

The other person in the chat demos is
[`sdk/go/examples/demo-bot`](../../sdk/go/examples/demo-bot). It creates the room,
and the script types its join code into the tape. Tapes that use
`{{JOIN_CODE}}` get a bot that replies; tapes that use `{{JOIN_CODE_SILENT}}` get
one that stays quiet.

### Web demos

`record-web.sh` runs Playwright and ffmpeg in containers, so you only need
Docker, Go and pnpm. The browser calls the API with credentials, so start the
API with the overlay that allows the demo origin:

```bash
docker compose -f docker-compose.yml -f docs/demos/web-demo.compose.yml --profile app up -d api
bash docs/demos/record-web.sh              # chat, reactions, tour -> docs/assets/demos/web-*.gif
bash docs/demos/record-web.sh reactions    # just this one
```

The scenes are in [`web/scenes.mjs`](web/scenes.mjs). They are captured as
lossless screenshots (about 10 per second) rather than Playwright's video, whose
compression noise makes the GIFs several times larger.
