#!/usr/bin/env bash
set -euo pipefail

# Supply Chain Hardening — リポジトリ現状スキャン
# Phase 1-1 で使用する調査スクリプト。
# リポジトリルートから実行すること。

section() {
	printf '\n## %s\n\n' "$1"
}

print_or() {
	if [ -n "$2" ]; then
		printf '%s\n' "$2"
	else
		echo "$1"
	fi
}

section "GitHub Actions ワークフロー一覧"
find .github/workflows -name '*.yml' 2>/dev/null | sort | head -30 ||
	echo "(GitHub Actions ワークフローなし)"

section "未ピン留めの外部 Action"
grep -rh 'uses:' .github/workflows/ 2>/dev/null |
	sed 's/.*uses: *//' |
	grep -v '#' |
	grep -v '^\.\/' |
	sort -u ||
	echo "(未ピン留め Action なし)"

section "ピン留め済みの Action"
grep -rh 'uses:' .github/workflows/ 2>/dev/null |
	sed 's/.*uses: *//' |
	grep '#' |
	sort -u |
	head -20 ||
	echo "(なし)"

section "Dependabot / Renovate 設定の存在"
print_or "(依存関係更新ツールの設定なし)" \
	"$(ls -la .github/dependabot.yml .github/dependabot.yaml \
		renovate.json renovate.json5 .renovaterc .renovaterc.json 2>/dev/null || true)"

section "Dependabot の cooldown 設定状況"
grep -A2 'cooldown' .github/dependabot.yml 2>/dev/null ||
	echo "(cooldown 未設定または dependabot.yml なし)"

section "Renovate の minimumReleaseAge 設定状況"
grep 'minimumReleaseAge' \
	renovate.json renovate.json5 .renovaterc .renovaterc.json 2>/dev/null ||
	echo "(minimumReleaseAge 未設定または renovate 設定なし)"

section "自動マージ"
print_or "(自動マージのワークフローなし)" \
	"$(grep -rln -e 'gh pr merge' -e 'enable-pull-request-automerge' .github/workflows/ 2>/dev/null || true)"
print_or "(dependabot/fetch-metadata 未使用)" \
	"$(grep -rn 'dependabot/fetch-metadata' .github/workflows/ 2>/dev/null || true)"

section "zizmor"
print_or "(zizmor 設定なし)" \
	"$(ls .github/zizmor.yml .github/zizmor.yaml 2>/dev/null || true)"
print_or "(CI で zizmor を使っていない)" \
	"$(grep -rln 'zizmor' .github/workflows/ mise.toml Makefile package.json 2>/dev/null || true)"

section "リポジトリ設定"
if ! repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null); then
	echo "(gh でリポジトリを特定できない)"
	exit 0
fi
gh api "repos/$repo/actions/permissions" \
	--jq '"sha_pinning_required: \(.sha_pinning_required)"' 2>/dev/null ||
	echo "(Actions の権限設定を取得できない)"
gh api "repos/$repo/actions/permissions/workflow" \
	--jq '"default_workflow_permissions: \(.default_workflow_permissions)", "can_approve_pull_request_reviews: \(.can_approve_pull_request_reviews)"' 2>/dev/null ||
	echo "(既定トークンの設定を取得できない)"
branch=$(gh repo view --json defaultBranchRef --jq .defaultBranchRef.name)
gh api "repos/$repo/branches/$branch/protection" \
	--jq '"enforce_admins: \(.enforce_admins.enabled)", "required_checks: \([.required_status_checks.checks[]?.context] | join(", "))"' 2>/dev/null ||
	echo "($branch のブランチ保護なし、または権限不足)"
