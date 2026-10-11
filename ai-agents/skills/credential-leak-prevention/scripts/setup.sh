#!/usr/bin/env bash
set -euo pipefail

# Credential Leak Prevention — セットアップ（冪等）
# リポジトリルートから実行すること。

ok() { printf '✓ %s\n' "$1"; }
info() { printf '→ %s\n' "$1"; }
err() {
	printf 'ERROR: %s\n' "$1" >&2
	exit 1
}

# 前提ツール確認
if ! command -v pre-commit >/dev/null 2>&1 || ! command -v gitleaks >/dev/null 2>&1; then
	err "pre-commit と gitleaks の両方がインストールされている必要があります。
  brew install pre-commit gitleaks"
fi

# .pre-commit-config.yaml
if [ ! -f .pre-commit-config.yaml ]; then
	info ".pre-commit-config.yaml を新規作成します"
	cat >.pre-commit-config.yaml <<'YAML'
repos:
  - repo: https://github.com/gitleaks/gitleaks
    rev: v8.30.1
    hooks:
      - id: gitleaks
YAML
	ok ".pre-commit-config.yaml を作成しました"
elif grep -q 'gitleaks' .pre-commit-config.yaml; then
	ok ".pre-commit-config.yaml に gitleaks hook が設定済みです（スキップ）"
else
	info "既存の .pre-commit-config.yaml に gitleaks hook を追記します"
	cat >>.pre-commit-config.yaml <<'YAML'
  - repo: https://github.com/gitleaks/gitleaks
    rev: v8.30.1
    hooks:
      - id: gitleaks
YAML
	ok "gitleaks hook を追記しました"
fi

# .gitleaks.toml
if [ -f .gitleaks.toml ]; then
	ok ".gitleaks.toml が存在します（スキップ）"
else
	info ".gitleaks.toml を作成します"
	{
		printf '[extend]\nuseDefault = true\n\n[allowlist]\npaths = [\n'
		printf "  '''%s''',\n" '\.gitleaks\.toml$'
		if [ -f go.sum ]; then
			printf "  '''%s''',\n" 'go\.sum$'
		fi
		printf ']\n'
	} >.gitleaks.toml
	ok ".gitleaks.toml を作成しました"
fi

# pre-commit install は .git/hooks に書き込み、サンドボックスでは拒否されるため、ここでは実行しない。
printf '\n設定ファイルの準備が完了しました。次にサンドボックス外で git hook を有効化してください:\n'
printf '  pre-commit install\n'
