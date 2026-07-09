#!/usr/bin/env bash
# Render a Marp deck to one or more formats (pdf / pptx / html).
#
# Usage: render_deck.sh <deck.md> [formats]
#   formats: comma-separated subset of pdf,pptx,html (default: pdf)
#
# Notes:
# - marp's `--pdf` and `--pptx` are mutually exclusive in a single invocation,
#   so each format is rendered in its own marp call.
# - Stale `marp-cli-*` temp dirs (leftover Chromium userDataDir locks) are
#   cleaned before each call to avoid "browser is already running" errors.
# - PDF/PPTX require headless Chromium. If Chromium fails to launch, the caller
#   is likely inside a sandbox that blocks it; re-run outside the sandbox.
set -euo pipefail

if [ "$#" -lt 1 ]; then
	echo "usage: render_deck.sh <deck.md> [formats]  (formats: pdf,pptx,html)" >&2
	exit 2
fi

DECK="$1"
FORMATS="${2:-pdf}"

if [ ! -f "$DECK" ]; then
	echo "error: deck not found: $DECK" >&2
	exit 1
fi

if ! command -v npx >/dev/null 2>&1; then
	echo "error: npx (Node.js) is required but not found on PATH" >&2
	exit 1
fi

TMPBASE="${TMPDIR:-/tmp}"
MARP=(npx --yes @marp-team/marp-cli@latest)

clean_stale_locks() {
	# Remove leftover marp Chromium userDataDir temp dirs (best effort).
	find "$TMPBASE" -maxdepth 1 -name 'marp-cli-*' -type d -exec rm -rf {} + 2>/dev/null || true
}

IFS=',' read -r -a FMT_ARR <<<"$FORMATS"
for fmt in "${FMT_ARR[@]}"; do
	fmt="$(echo "$fmt" | tr -d '[:space:]')"
	case "$fmt" in
	pdf | pptx | html) ;;
	"") continue ;;
	*)
		echo "error: unsupported format '$fmt' (use pdf, pptx, or html)" >&2
		exit 2
		;;
	esac
	clean_stale_locks
	echo ">> rendering $fmt ..."
	"${MARP[@]}" "$DECK" "--$fmt" --allow-local-files
done

echo ">> done. outputs are next to $DECK"
