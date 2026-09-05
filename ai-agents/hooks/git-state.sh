#!/usr/bin/env bash
# UserPromptSubmit hook: 現在の git 作業状態（ブランチ / 変更数 / upstream 差分 /
# 進行中オペレーション / linked worktree）を additionalContext として注入する。
# Claude が組み立てる git コンテキストはセッション開始時の 1 回きりなので、
# 途中のブランチ切り替え・コミット・中断した rebase 等でモデルの前提だけが古くなる。
# stdin: hook JSON (.cwd を使う)。stdout: hookSpecificOutput.additionalContext。
# 読み取り専用・ネットワークアクセスなし（fetch しない）。
# ファイル名は出さず件数だけにして、毎プロンプトのトークンコストを固定・最小に保つ。
# 情報提供専用なので決してブロックしない。exit 2 はプロンプト自体を破棄するため、
# どの失敗パスでも黙って exit 0 に落ちる（fail open）。
set -u

INPUT=$(cat)
command -v jq >/dev/null 2>&1 || exit 0

cwd=$(printf '%s' "$INPUT" | jq -r '.cwd // empty' 2>/dev/null)
{ [ -n "$cwd" ] && [ -d "$cwd" ] && cd "$cwd"; } || exit 0
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0

# --branch 付き porcelain を 1 回だけ叩き、upstream 差分と変更数をまとめて得る。
status=$(git status --porcelain=v1 --branch --untracked-files=normal 2>/dev/null) || exit 0
head_line=$(printf '%s\n' "$status" | head -n 1) # ## feat/foo...origin/feat/foo [ahead 2, behind 1]
body=$(printf '%s\n' "$status" | tail -n +2)

branch=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || echo "detached@$(git rev-parse --short HEAD)")
staged=$(printf '%s\n' "$body" | grep -c '^[MTADRC]' || true)
unstaged=$(printf '%s\n' "$body" | grep -c '^.[MTD]' || true)
untracked=$(printf '%s\n' "$body" | grep -c '^??' || true)
track=$(printf '%s' "$head_line" | sed -n 's/.*\[\(.*\)\]$/\1/p')

lines="git: branch=$branch staged=$staged unstaged=$unstaged untracked=$untracked${track:+ ($track)}"

# linked worktree なら実体パスも出す（worktree-create.sh 運用との組み合わせ）。
if [ "$(git rev-parse --git-common-dir)" != "$(git rev-parse --git-dir)" ]; then
	lines="$lines"$'\n'"worktree: $(git rev-parse --show-toplevel) (linked)"
fi

# 中断された rebase/merge 等は status の見た目から分かりにくく、モデルからは不可視。
gitdir=$(git rev-parse --git-dir)
op=""
[ -e "$gitdir/MERGE_HEAD" ] && op="merge"
{ [ -d "$gitdir/rebase-merge" ] || [ -d "$gitdir/rebase-apply" ]; } && op="rebase"
[ -e "$gitdir/CHERRY_PICK_HEAD" ] && op="cherry-pick"
[ -e "$gitdir/REVERT_HEAD" ] && op="revert"
[ -e "$gitdir/BISECT_LOG" ] && op="bisect"
[ -n "$op" ] && lines="$lines"$'\n'"WARNING: $op in progress — 完了/中止するまで他の git 操作をしない"

default=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || true)
default="${default#origin/}"
if [ -n "$default" ] && [ "$branch" = "$default" ] && [ $((staged + unstaged + untracked)) -gt 0 ]; then
	lines="$lines"$'\n'"WARNING: default ブランチ ($default) 上に変更あり — コミット前にブランチを切る"
fi

jq -n --arg ctx "$lines" \
	'{hookSpecificOutput:{hookEventName:"UserPromptSubmit",additionalContext:$ctx}}'
exit 0
