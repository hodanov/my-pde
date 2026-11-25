# Plan: lint-changed の markdownlint 設定統一と同一報告の抑止

## Background

PR #996 で Stop フック（`lint-changed.sh`）の報告が `hookSpecificOutput.additionalContext` 経由でモデルに届くようになった。
それ以前は Stop の plain stdout が debug ログにしか残らず、フックの誤検知は誰にも見えなかった。
報告が「ターンを継続させるコンテキスト」になったことで、潜在していた誤検知が毎ターンの追加ラウンドとして表面化した。

実例: markdownlint を使っていない別リポの未追跡 doc（日本語）が毎回報告される。
調査で原因を特定済み。

- 174 件すべてが `MD013/line-length`（80 桁）。日本語 doc では常に超過する。
- `markdown-format.sh`（PostToolUse）は `--config "$SCRIPT_DIR/.markdownlint-cli2.yaml"`（`MD013: false`, `MD041: false`）を渡す。
  `lint-changed.sh` は `markdownlint-cli2 "$f"` と素で呼ぶ。そのリポには markdownlint の設定も CI も無く、デフォルト規則が当たる。
- sidecar 設定を当てると同ファイルは 0 件になる（実測済み）。
- 実機でフックを再現し、`[markdownlint] <対象 doc>` が 1 件返ることを確認済み。

狙う成果: 自分の設定で整形済みのファイルを lint 側が別基準で落とす不整合を無くし、直せない既存違反が将来出ても同一報告で毎ターン往復しない。

## Current structure

- `ai-agents/hooks/lint-changed.sh`: unstaged + staged + untracked の union を拡張子で振り分け、stylua / markdownlint / shellcheck / actionlint / hadolint / golangci-lint を実行。指摘があれば `additionalContext` で出力。`stop_hook_active` が true なら即終了。
- 配布: `mise run claude-settings-copy` が `ai-agents/hooks` を `~/.claude/hooks/` へ、`ai-agents/settings/claude` を `~/.claude` へ `--force` でコピーする。sidecar `.markdownlint-cli2.yaml` は `~/.claude/hooks/` に配布済み。
- 配線: `ai-agents/settings/claude/settings.json` の Stop（`~/.claude/hooks/lint-changed.sh`）。`ai-agents/hooks/hooks.json` にもあるが、リポの `enabledPlugins` には `ai-agents@my-pde` が無い。ユーザーレベル settings は未確認。
- テスト: `lint-changed.sh` 専用の自動テストは無い。`verify-changed.sh` が shfmt / shellcheck をかける。#996 も手動確認で検証している。

## Design policy

専門家の手順は「測る → 検査の契約を 1 箇所にする → 報告を変化量にだけ反応させる」。

1. **測る**: 再現とルール別内訳で原因を確定する（済み。MD013 が 174/174）。
2. **契約を 1 箇所に**: markdownlint の設定を `markdown-format.sh` と同じ sidecar ファイルに一本化する。2 つのフックが別基準で同じファイルを裁く状態をなくす。パス解決は hook-authoring ルールどおり `SCRIPT_DIR` 相対にし、plugin 経路でも同じ組が動くようにする。
3. **報告を変化量にだけ反応させる**: セッション内で直前の報告と同一なら再送しない。状態は `${TMPDIR:-/tmp}` に session_id 単位で置く。指摘が 0 になったら状態を消し、再発したら再び報告する。同一ペイロードの比較だけで足り、ハッシュも抽象も足さない。
4. **採らない案**: セッション編集ファイルへの限定（PostToolUse で編集パスを記録し Stop で交差）は最も根本的だが、新規フックと 2 経路の配線が要り、Bash 経由の変更を取りこぼす。設定統一 + 同一報告の抑止で足りなければ次の一手にする。

## Implementation steps

1. `lint-changed.sh` に `SCRIPT_DIR` を追加し、markdownlint 呼び出しを `markdownlint-cli2 --config "$SCRIPT_DIR/.markdownlint-cli2.yaml" "$f"` にする。
2. stdin の `session_id` を `jq -r '.session_id // empty'` で取得する。
3. 末尾で状態ファイル `${TMPDIR:-/tmp}/claude-lint-changed-${session_id}` を扱う。
   - `session_id` が空: 抑止せず従来どおり報告。
   - `issues` が空: 状態ファイルを削除して終了。
   - `issues` が状態ファイルの内容と同一: 何も出さず終了。
   - それ以外: 状態ファイルへ `issues` を書き、`additionalContext` を出力。
4. `./ai-agents/scripts/verify-changed.sh` で shfmt / shellcheck を通す。
5. `docs-sync` で、`lint-changed.sh` の挙動を書いた箇所（AGENTS.md、`dev-workflow` / `hook-scaffold` の SKILL.md）が追随不要か確認する。いずれも「失敗をツール名つきで報告する」という記述で、変更後も成り立つ。
6. 反映は、コミット後に `ai-agents/settings/claude` に未コミット変更が無い状態で `mise run claude-settings-copy` を実行する。`--force` で `settings.json` ごと配るため、作業中の設定を巻き込まない。

## File changes

| File                                                                 | Change                                 |
| -------------------------------------------------------------------- | -------------------------------------- |
| `ai-agents/hooks/lint-changed.sh`                                    | sidecar 設定の受け渡しと同一報告の抑止 |
| `docs/plan/2026-11-25_lint-changed-markdownlint-config-and-dedup.md` | この計画の保存                         |

`hooks.json` / `settings.json` の配線、`markdown-format.sh`、`.claude/hooks/test-changed.sh` は変更しない。

## Risks and mitigations

| Risk                                                         | Mitigation                                                                                           |
| ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| 同一報告の抑止で、直っていない違反をモデルが再び見ない       | 抑止するのは直前と完全一致のときだけ。ファイルの増減や別の指摘が混ざればペイロードが変わり再送される |
| 指摘を直した後に同じ指摘が再発しても抑止されたままになる     | 指摘が 0 になった時点で状態ファイルを削除するので、再発は新規として報告される                        |
| `session_id` が取れない環境で抑止が効かない                  | 従来どおり毎回報告するだけで、挙動は #996 時点と同じ。安全側に倒れる                                 |
| 2 経路が同時に発火した場合、状態ファイルの書き込みが競合する | 報告が 2 回出る可能性が残るだけで、破損しても次回に再送されるだけ。実害が出たら別途対処              |

## Validation

使い捨ての git リポで、フックの出力と状態ファイルを検証するスクリプトを使った。修正前の `HEAD` 版では「markdown の長い行」と「同一報告の抑止」の 2 項目が赤になり、修正後は 10 項目すべて緑になる。

- [x] 上記の別リポで `{"stop_hook_active":false,"session_id":"..."}` を流し、出力が無いこと（修正前は markdownlint の 1 件）。
- [x] 同じ `session_id` で 2 回実行し、1 回目は JSON、2 回目は無出力。
- [x] 違反を直すと無出力かつ状態ファイルが消える。同じ違反を戻すと再び報告される。
- [x] 別の `session_id` では同じ違反が再び報告される。
- [x] `stop_hook_active:true` と `session_id` 無しの挙動が従来どおり。
- [x] `shfmt -d` と `shellcheck` が `lint-changed.sh` で clean。
- [x] 一時ファイルと状態ファイルは検証後に削除済み。

## Open questions

- 日本語ファイル名の取りこぼし（別PRで判断）: `git ls-files --others` と `git diff --name-only` は非 ASCII のパスをクォートするため、`[ -f "$f" ]` で弾かれる。日本語名の `.md` は現状 lint されていない。`git -c core.quotePath=false` で直せるが、直すと sidecar 設定でも違反が残るファイルが新たに報告され、今回の目的と逆向きになる。
- `ai-agents@my-pde` plugin がユーザーレベルで有効なら、Stop の報告が 2 経路で二重に出ている可能性がある。未確認。
- セッション編集ファイル限定は、今回の対策後も既存違反のノイズが残る場合に再検討する。
