#!/usr/bin/env bash
# Re-records the demo GIFs in docs/assets/demos from the *.tape files here.
#
#   docker compose --profile app up -d --build   # the full stack must be running
#   bash docs/demos/record.sh                    # all demos
#   bash docs/demos/record.sh chat               # just docs/demos/chat.tape
#
# Each tape runs in the charmbracelet/vhs container, inside the api
# container's network namespace.
# Tapes containing {{JOIN_CODE}} get a fresh room from examples/demo-bot (a
# scripted second participant); tapes containing {{JOIN_CODE_SILENT}} get a
# room whose bot member never replies.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEMOS="docs/demos"
WORK="$DEMOS/.work" # git-ignored scratch: binaries and rendered tapes
API_CONTAINER="${VISPER_API_CONTAINER:-visper-api}"
VHS_IMAGE="${VHS_IMAGE:-ghcr.io/charmbracelet/vhs:v0.12.1}"

cd "$ROOT"
mkdir -p "$WORK" docs/assets/demos

echo "building linux binaries"
(cd clients/cli && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$ROOT/$WORK/visper" ./cmd/visper)
(cd sdk/go && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$ROOT/$WORK/demo-bot" ./examples/demo-bot)

tapes=("$@")
if [ ${#tapes[@]} -eq 0 ]; then
	for t in "$DEMOS"/*.tape; do tapes+=("$(basename "$t" .tape)"); done
fi

for name in "${tapes[@]}"; do
	echo "recording $name"
	sed "s/{{JOIN_CODE_SILENT}}/{{JOIN_CODE}}/g" "$DEMOS/$name.tape" > "$WORK/$name.tape"
	bot_flags=""
	grep -q "{{JOIN_CODE_SILENT}}" "$DEMOS/$name.tape" && bot_flags="-silent"
	needs_bot=0
	grep -q "{{JOIN_CODE" "$DEMOS/$name.tape" && needs_bot=1

	# The container shares the api container's network namespace, so the file
	# links the API hands out (http://localhost:5005/...) resolve exactly as on
	# the host, and compose service names (ai, ...) still resolve.
	# MSYS_NO_PATHCONV keeps Git Bash on Windows from rewriting the container paths.
	MSYS_NO_PATHCONV=1 docker run --rm \
		--network "container:$API_CONTAINER" \
		-v "$ROOT:/repo" -w /repo \
		-e VISPER_BASE_URL=http://localhost:5005 \
		-e VISPER_AI_BASE_URL=http://ai:8088 \
		-e NEEDS_BOT="$needs_bot" -e BOT_FLAGS="$bot_flags" -e NAME="$name" -e WORK="$WORK" \
		--entrypoint bash "$VHS_IMAGE" -c '
			set -euo pipefail
			install -m 0755 "$WORK/visper" /usr/local/bin/visper
			# Clean HOME per recording: fresh CLI settings, and pictures for the file demo.
			export HOME=/tmp/home
			mkdir -p "$HOME/Pictures"
			cp clients/web/public/logo512.png "$HOME/Pictures/visper-logo.png"
			cp clients/cli/internal/tui/embeds/waifu.png "$HOME/Pictures/sketch.png"
			if [ "$NEEDS_BOT" = 1 ]; then
				rm -f /tmp/code
				"$WORK/demo-bot" -code-file /tmp/code -user-id "demo-bot-$NAME-$RANDOM" $BOT_FLAGS 2>/tmp/bot.log &
				for _ in $(seq 50); do [ -s /tmp/code ] && break; sleep 0.2; done
				[ -s /tmp/code ] || { cat /tmp/bot.log; exit 1; }
				sed -i "s/{{JOIN_CODE}}/$(cat /tmp/code)/g" "$WORK/$NAME.tape"
			fi
			vhs "$WORK/$NAME.tape"
		'
done

echo "done: docs/assets/demos/"
