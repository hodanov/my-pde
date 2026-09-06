#!/bin/bash
# PreToolUse(Bash) guard: 取り返しのつかない破壊的コマンドを実行前にブロックする。
# exit 2 + stderr で Claude はツール呼び出しを中断し、理由をモデルに返す。
# 悪意ある回避の完全防御ではなく「うっかり事故の防止」を目的とした多層防御の 1 枚。
set -u

INPUT=$(cat)

CMD=$(printf '%s' "$INPUT" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
except Exception:
    print('')
    sys.exit(0)
print(data.get('tool_input', {}).get('command', ''))
")

[ -n "$CMD" ] || exit 0

block() {
	echo "[guard-dangerous-bash] blocked: $1" >&2
	echo "危険な操作を検出したためブロックしました。意図的な場合はユーザーに確認するか、手動で実行してください。" >&2
	exit 2
}

# 改行を空白へ、連続空白を 1 つに正規化してからパターン照合する。
norm=$(printf '%s' "$CMD" | tr '\n' ' ' | tr -s ' ')

case "$norm" in
*"rm -rf /"* | *"rm -rf ~"*)
	block "rm -rf on a dangerous path"
	;;
*"git reset --hard"*)
	block "git reset --hard (discards uncommitted work)"
	;;
*"git clean -"*d*f* | *"git clean -"*f*d*)
	block "git clean -fd (deletes untracked files)"
	;;
*"git push"*"--force"* | *"git push"*" -f"*)
	block "git push --force"
	;;
*"--no-verify"*)
	block "--no-verify (bypasses pre-commit/secret guards)"
	;;
*"git checkout ."* | *"git restore ."*)
	block "mass discard of working-tree changes"
	;;
*":(){ :|:& };:"*)
	block "fork bomb"
	;;
esac

exit 0
