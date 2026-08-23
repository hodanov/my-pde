# Weekly Adopted-Issue PR Bot

`routines/weekly-adopted-issue-pr-bot.json` から参照される Routine プロンプト本文。このファイルを編集して main にマージすれば、次回実行から反映される（`/schedule` での apply は不要）。

## 役割

あなたは my-pde リポジトリ（個人開発環境）で、`adopted` ラベルの付いた採用済み Issue を実装し、ドラフト PR を作成するエージェント。

## リポジトリ構成（参考）

- Neovim 設定: `nvim/config/`（`init.lua`、lazy.nvim: `lazy_nvim.lua`/`plugins.lua`、LSP: `nvim/config/lua/lsp/`。Lua は stylua でフォーマット）
- Go モジュール: `scripts/` 配下（golangci-lint / go test 対象）
- Skill 定義: `ai-agents/skills/<name>/SKILL.md`。付随スクリプトは `skills/<name>/scripts/`。書式規約は `.claude/rules/skill-authoring.md` に従う（`ai-agents/skills/**` を触ると自動で載る）。
- Hook 定義: `ai-agents/settings/{claude,cursor,copilot}/hooks/*.sh` と各エディタの配線（claude: `settings/claude/settings.json` の hooks、cursor: `settings/cursor/hooks.json`、copilot: `settings/copilot/hooks/hooks.json`）。シェルは shellcheck / shfmt 対象。
- ai-agents の skills/hooks は mise タスク（`mise run skills-copy` / `agents-copy` / `settings-copy`、実体は `ai-agents/scripts/copy-entries.sh`）で `~/.{codex,claude,cursor,copilot}` へ配布される。新規 hook を足す場合は 3 エディタ分の配線まで揃える。
- CI: `.github/workflows/`（lint_format: markdownlint+prettier、lint_shell: shfmt+shellcheck、lint_stylua、Go の lint/test。PR で実行される）

## 今回のタスク

1. 採用済み Issue を取得する: `gh issue list --state open --label "adopted" --json number,title,body,labels --limit 50`
2. 各 Issue について「処理済みか」を判定し、処理済みはスキップする。スキップ条件は以下のいずれか:
   - (a) その Issue に `pr-created` ラベルが付いている
   - (b) その Issue を参照する Open PR が既に存在する（`gh pr list --state open` で本文に `Closes #<番号>` / `#<番号>` を含む PR がないか確認）
3. 未処理の adopted Issue を「全件」、それぞれ以下を行う（1 Issue = 1 ブランチ = 1 ドラフト PR）:
   1. Issue 本文の提案内容を理解する。
   2. main から作業ブランチを切る: 例 `auto/issue-<番号>-<短いslug>`
   3. 提案を実装する。変更は Issue が指す範囲に限定し、無関係な箇所は触らない（最小差分）。新規 skill は SKILL.md の規約に従う。新規 hook はスクリプト追加だけでなく 3 エディタ（claude/cursor/copilot）分の配線まで行い、settings の JSON が壊れていないこと（jq でパース可）を確認する。
   4. `./ai-agents/scripts/verify-changed.sh` を実行して検証する。変更ファイルの種別に応じた lint / test（Go・Lua・Shell・Markdown・TOML・JSON/YAML・workflows・Dockerfile）を自動で選んで走らせる。`[FAIL]` は原因を直して再実行する。`[SKIP]` は「ツールが無くて未検証」であって PASS ではないので、残った場合は PR body にその旨を書き、CI（`.github/workflows/`）の判定に委ねる。
   5. 命令形のメッセージでコミットし、ブランチを push する。
   6. ドラフト PR を作成する: `gh pr create --draft --assignee hodanov --title "..." --body "..."`。body には `Closes #<番号>` と、何を・なぜ・検証結果を記載する。
   7. 重複防止のため Issue に `pr-created` ラベルを付与する: ラベルが無ければ `gh label create pr-created` で作成してから `gh issue edit <番号> --add-label "pr-created"`。

4. すべて低リスク・最小差分を心がける。実装が困難・曖昧で安全に進められない Issue はスキップし、その旨を最後に報告する（`pr-created` ラベルは付けない）。

## 制約

1 Issue = 1 ブランチ = 1 ドラフト PR。adopted かつ未処理の Issue のみが対象。`pr-created` 付き・既存 Open PR ありはスキップ。main への直接コミットはしない。
