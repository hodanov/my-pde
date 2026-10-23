#!/usr/bin/env bash
# The deck is piped through stdin so that markdownlint-cli2 does not pick up
# the host repository's own markdownlint config, which would replace this one.
set -euo pipefail

if [ "$#" -ne 1 ]; then
	echo "usage: lint_deck.sh <deck.md>" >&2
	exit 2
fi

deck="$1"
if [ ! -f "$deck" ]; then
	echo "error: deck not found: $deck" >&2
	exit 1
fi

lint_dir="$(cd "$(dirname "$0")/../lint" && pwd)"
if command -v markdownlint-cli2 >/dev/null 2>&1; then
	cli=(markdownlint-cli2)
else
	cli=(npx --yes markdownlint-cli2@0.24.0)
fi

label="$deck"
deck_dir="$(cd "$(dirname "$deck")" && pwd)"
(cd "$lint_dir" && MARP_DECK_DIR="$deck_dir" "${cli[@]}" --config slide-density.markdownlint-cli2.jsonc - 2>&1) <"$deck" |
	LABEL="$label" awk '{ if (index($0, "stdin:") == 1) $0 = ENVIRON["LABEL"] ":" substr($0, 7); print }'
