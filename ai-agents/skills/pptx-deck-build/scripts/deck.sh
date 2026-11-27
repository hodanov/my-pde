#!/usr/bin/env bash
set -eu

skill_dir="$(cd "$(dirname "$0")/.." && pwd)"
export PYTHONPATH="$skill_dir/scripts"
export PYTHONDONTWRITEBYTECODE=1
exec uv run --no-project --python '>=3.12' --exclude-newer 2026-11-13T00:00:00Z \
	--with python-pptx==1.0.2 --with pyyaml==6.0.3 --with pymupdf==1.28.2 \
	python -m deckbuild "$@"
