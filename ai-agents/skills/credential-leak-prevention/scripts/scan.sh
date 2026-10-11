#!/usr/bin/env bash
set -euo pipefail

# Credential Leak Prevention — 現状スキャン
# リポジトリルートから実行すること。

ok() { printf '  ✓  %s\n' "$1"; }
ng() { printf '  ✗  %s\n' "$1"; }
section() { printf '\n## %s\n\n' "$1"; }

section "ツールのインストール状況"
if command -v pre-commit >/dev/null 2>&1; then
	ok "pre-commit: $(pre-commit --version 2>&1 | head -1)"
else
	ng "pre-commit: 未インストール"
fi

if command -v gitleaks >/dev/null 2>&1; then
	ok "gitleaks: $(gitleaks version 2>&1 | head -1)"
else
	ng "gitleaks: 未インストール"
fi

section ".pre-commit-config.yaml"
if [ -f .pre-commit-config.yaml ]; then
	ok ".pre-commit-config.yaml が存在する"
	if grep -q 'gitleaks' .pre-commit-config.yaml; then
		ok "gitleaks hook が設定済み"
	else
		ng "gitleaks hook が未設定"
	fi
else
	ng ".pre-commit-config.yaml が存在しない"
fi

section ".gitleaks.toml"
if [ -f .gitleaks.toml ]; then
	ok ".gitleaks.toml が存在する"
	if grep -q 'useDefault = true' .gitleaks.toml; then
		ok "useDefault = true が設定済み"
	else
		ng "useDefault = true が未設定（デフォルトルールが無効になっている可能性あり）"
	fi
else
	ng ".gitleaks.toml が存在しない（gitleaks のデフォルトルールで動作する）"
fi

section "git hook 有効化状況"
hooks_path=$(git config --get core.hooksPath || true)
if [ -n "$hooks_path" ]; then
	ng "core.hooksPath = ${hooks_path} が設定済み（pre-commit install は拒否される。既存の hook 管理との統合方針をユーザーに確認する）"
fi

pre_commit_hook="$(git rev-parse --git-path hooks)/pre-commit"
if [ -f "$pre_commit_hook" ]; then
	ok "git hook: ${pre_commit_hook} が存在する（pre-commit install 済み）"
else
	ng "git hook: ${pre_commit_hook} が存在しない（pre-commit install が必要）"
fi
