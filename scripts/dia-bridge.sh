#!/bin/bash
# Open the bridge in Dia's agent chat.
#
# The gateway ccw dials is built by Dia's handler for one chat turn, and the
# bridge message is the only way to make a new one: the token it carries lives
# inside that handler. Every call to ccw's dia provider answers 503 until this
# has run.
#
# The key order is the whole trick, and it was found by watching Dia rather than
# by reading it:
#   * space first, because "bridge" typed on its own submits as a browser
#     omnibox query and ends up in Google instead of the chat,
#   * down arrow next, because space alone selects the completion but leaves the
#     field in browse mode, so the turn is never sent as a chat turn,
#   * return last, which sends it.
#
# Skipping the space leaves the gateway file without a token and every dia call
# answers 401; skipping the arrow leaves the turn unsent.
#
# Needs Accessibility for osascript, a one-time grant in System Settings. When
# that is missing, ccw answers 503 with the same instruction and this is the
# by-hand path.
#
# Usage: bash scripts/dia-bridge.sh
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATEWAY_FILE="${DIA_GATEWAY_FILE:-/tmp/dia-handler.json}"
say() { printf '%s\n' "$*" >&2; }

# The gateway file is what the bridge reads on every request, so a stale one from
# a handler that has since exited is removed first. Otherwise the wait below sees
# the old port and reports a bridge that is not there.
rm -f "$GATEWAY_FILE"

if ! pgrep -x Dia >/dev/null; then
  say "Dia is not running; start it first"
  exit 1
fi

say "typing the bridge word"
osascript "$SCRIPT_DIR/dia-bridge.applescript" >/dev/null 2>&1

for i in $(seq 1 20); do
  sleep 1
  if [ -f "$GATEWAY_FILE" ]; then
    if grep -q '"token":"[^"]' "$GATEWAY_FILE" 2>/dev/null; then
      say "gateway is up: $(head -c 60 "$GATEWAY_FILE")"
      exit 0
    fi
    # The file appears as soon as the handler binds the port, before the token is
    # minted, so a tokenless file is a turn still in flight rather than a failure.
    say "port up, waiting for the token ($i s)"
  fi
done

say "no gateway with a token yet; in Dia type 'bridge', press space, then down, then return"
exit 1
