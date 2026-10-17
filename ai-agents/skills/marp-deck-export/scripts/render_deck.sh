#!/usr/bin/env bash
# Render a Marp deck to one or more formats (pdf / pptx / html).
#
# Usage: render_deck.sh <deck.md> [formats]
#   formats: comma-separated subset of pdf,pptx,html,png (default: pdf)
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
	echo "usage: render_deck.sh <deck.md> [formats]  (formats: pdf,pptx,html,png)" >&2
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

BRAVE="/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
if [ -z "${CHROME_PATH:-}" ] && [ ! -d "/Applications/Google Chrome.app" ] && [ -x "$BRAVE" ]; then
	export CHROME_PATH="$BRAVE"
fi

TMPBASE="${TMPDIR:-/tmp}"
THEMES="$(cd "$(dirname "$0")/../assets/themes" && pwd)"
MARP=(npx --yes @marp-team/marp-cli@latest)
# --no-stdin: marp waits for Markdown on a non-TTY stdin, which hangs in agent shells.
MARP_OPTS=(--no-stdin --theme-set "$THEMES" --allow-local-files)

clean_stale_locks() {
	# Remove leftover marp Chromium userDataDir temp dirs (best effort).
	find "$TMPBASE" -maxdepth 1 -name 'marp-cli-*' -type d -exec rm -rf {} + 2>/dev/null || true
}

IFS=',' read -r -a FMT_ARR <<<"$FORMATS"
for fmt in "${FMT_ARR[@]}"; do
	fmt="$(echo "$fmt" | tr -d '[:space:]')"
	case "$fmt" in
	pdf | pptx | html | png) ;;
	"") continue ;;
	*)
		echo "error: unsupported format '$fmt' (use pdf, pptx, html, or png)" >&2
		exit 2
		;;
	esac
	clean_stale_locks
	echo ">> rendering $fmt ..."
	if [ "$fmt" = png ]; then
		png_dir="$(mktemp -d "$TMPBASE/marp-png.XXXXXX")"
		"${MARP[@]}" "$DECK" "${MARP_OPTS[@]}" --images png -o "$png_dir/$(basename "${DECK%.md}").png"
		echo ">> png slides: $png_dir"
		if command -v magick >/dev/null 2>&1; then
			# montage exits non-zero when no font is configured, even though the sheets are written.
			magick montage "$png_dir"/*.png -tile 3x3 -geometry 640x360+4+4 -background '#888888' "$png_dir/sheet-%d.png" 2>/dev/null || true
			for sheet in "$png_dir"/sheet-*.png; do
				if [ -f "$sheet" ]; then
					echo ">> contact sheet (9 slides each): $sheet"
				fi
			done
		fi
	else
		"${MARP[@]}" "$DECK" "${MARP_OPTS[@]}" "--$fmt"
	fi
done

echo ">> done. pdf/pptx/html outputs are next to $DECK"
