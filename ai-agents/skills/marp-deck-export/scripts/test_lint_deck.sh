#!/usr/bin/env bash
set -eu

skill_dir="$(cd "$(dirname "$0")/.." && pwd)"
fixtures="$skill_dir/lint/fixtures"
lint="$skill_dir/scripts/lint_deck.sh"
status=0

if out=$("$lint" "$fixtures/over-budget.md"); then
	echo "FAIL: over-budget.md passed the lint"
	status=1
fi
actual=$(printf '%s\n' "$out" | grep ' slide-density ' | sed "s|^$fixtures/||" || true)
if ! diff <(printf '%s\n' "$actual") "$fixtures/over-budget.expected"; then
	echo "FAIL: over-budget.md violations differ from over-budget.expected (< actual, > expected)"
	status=1
fi

for deck in "$fixtures/handout.md" "$skill_dir/assets/marp-template.md"; do
	if ! out=$("$lint" "$deck"); then
		printf 'FAIL: %s\n%s\n' "$deck" "$out"
		status=1
	fi
done

if [ "$status" -eq 0 ]; then
	echo "PASS"
fi
exit "$status"
