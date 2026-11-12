#!/usr/bin/env bash
# Re-records the web client demo GIFs in docs/assets/demos (web-*.gif).
#
#   docker compose -f docker-compose.yml -f docs/demos/web-demo.compose.yml --profile app up -d api
#   bash docs/demos/record-web.sh              # all scenes
#   bash docs/demos/record-web.sh chat         # one scene (chat, reactions, tour)
#
# The API must allow the web origin for credentialed requests, hence the
# compose overlay. Playwright records the production build of clients/web in
# its own container, inside the api container's network namespace, so
# localhost:5005 is the API and localhost:3000 the page, as in development.
# The scenes are captured as lossless screenshots; ffmpeg (also in a
# container) turns them into GIFs. The other person in
# the chat scenes is sdk/go/examples/demo-bot.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEMOS="docs/demos"
WORK="$DEMOS/.work" # git-ignored scratch
API_CONTAINER="${VISPER_API_CONTAINER:-visper-api}"
PLAYWRIGHT_VERSION="${PLAYWRIGHT_VERSION:-1.49.1}"
PLAYWRIGHT_IMAGE="mcr.microsoft.com/playwright:v$PLAYWRIGHT_VERSION-noble"
FFMPEG_IMAGE="${FFMPEG_IMAGE:-mwader/static-ffmpeg:7.1}"

cd "$ROOT"
mkdir -p "$WORK" docs/assets/demos

scenes=("$@")
[ ${#scenes[@]} -eq 0 ] && scenes=(chat reactions tour)

echo "building the demo bot"
(cd sdk/go && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$ROOT/$WORK/demo-bot" ./examples/demo-bot)

echo "building the web client"
(cd clients/web && VITE_BACKEND_API_URL=http://localhost:5005 pnpm build >/dev/null)

echo "recording ${scenes[*]}"
# MSYS_NO_PATHCONV keeps Git Bash on Windows from rewriting the container paths.
MSYS_NO_PATHCONV=1 docker run --rm \
	--network "container:$API_CONTAINER" \
	--ipc=host \
	-v "$ROOT:/repo" -w /repo \
	-e VISPER_BASE_URL=http://localhost:5005 \
	-e WORK="$WORK" -e SCENES="${scenes[*]}" -e PLAYWRIGHT_VERSION="$PLAYWRIGHT_VERSION" \
	"$PLAYWRIGHT_IMAGE" bash -c '
		set -euo pipefail
		# Colour emoji for the reactions.
		apt-get -qq update >/dev/null && apt-get -qq install -y fonts-noto-color-emoji >/dev/null
		mkdir -p /tmp/pw && cp docs/demos/web/scenes.mjs /tmp/pw/
		(cd /tmp/pw && npm init -y >/dev/null && npm i --silent --no-audit --no-fund "playwright@$PLAYWRIGHT_VERSION")

		PORT=3000 node clients/web/.output/server/index.mjs >/tmp/web.log 2>&1 &
		for _ in $(seq 50); do
			node -e "fetch(\"http://localhost:3000/\").then(r => process.exit(r.ok ? 0 : 1), () => process.exit(1))" && break
			sleep 0.2
		done

		for scene in $SCENES; do
			echo "  $scene"
			code=""
			if [ "$scene" != tour ]; then
				rm -f /tmp/code
				"$WORK/demo-bot" -code-file /tmp/code -user-id "demo-bot-web-$scene-$RANDOM" 2>/tmp/bot.log &
				bot=$!
				for _ in $(seq 50); do [ -s /tmp/code ] && break; sleep 0.2; done
				[ -s /tmp/code ] || { cat /tmp/bot.log; exit 1; }
				code=$(cat /tmp/code)
			fi
			(cd /tmp/pw && SCENE="$scene" INVITE_CODE="$code" OUT="/repo/$WORK" node scenes.mjs) || { cat /tmp/bot.log /tmp/web.log 2>/dev/null | tail -40; exit 1; }
			[ -n "$code" ] && kill "$bot" 2>/dev/null || true
		done
	'

echo "converting to GIF"
for scene in "${scenes[@]}"; do
	# Clean frames compress well: unchanged pixels are reused between frames.
	MSYS_NO_PATHCONV=1 docker run --rm -v "$ROOT:/repo" -w "/repo/$WORK/$scene-frames" "$FFMPEG_IMAGE" \
		-y -loglevel error -f concat -i frames.txt \
		-vf "fps=10,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=full[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" \
		"/repo/docs/assets/demos/web-$scene.gif"
	echo "  docs/assets/demos/web-$scene.gif ($(du -h "docs/assets/demos/web-$scene.gif" | cut -f1))"
done
