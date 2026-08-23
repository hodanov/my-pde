# Plan: Routine に repo の rules / ai-agents を効かせる

## Background

クラウドで動く Routine（`routines/*.json` の 8 本）に `.claude/rules/` や skills が効いているのか不明だった。公式ドキュメント（[Configure cloud environments](https://code.claude.com/docs/en/cloud-environments) の "What carries over from your setup"）で確認した結論は次の通り。

| 対象                                                                    | クラウドで有効 | 理由（原文）                                |
| ----------------------------------------------------------------------- | -------------- | ------------------------------------------- |
| repo の `CLAUDE.md`                                                     | Yes            | Part of the clone                           |
| repo の `.claude/rules/`                                                | Yes            | Part of the clone                           |
| repo の `.claude/skills/`, `.claude/agents/`, `.claude/commands/`       | Yes            | Part of the clone                           |
| repo の `.claude/settings.json` hooks                                   | Yes            | Part of the clone                           |
| repo の `.mcp.json`                                                     | Yes            | Part of the clone                           |
| `.claude/settings.json` で宣言した plugins                              | Yes            | セッション開始時に marketplace から install |
| user の `~/.claude/CLAUDE.md`                                           | No             | Lives on your machine, not in the repo      |
| user の `~/.claude/skills/`, `~/.claude/agents/`, `~/.claude/commands/` | No             | Live on your machine, not in the repo       |

つまり **repo にコミット済みの `.claude/**` は既に Routine でも効いている**（`guard-version-pins.sh` と `test-changed.sh` の hooks も発火している）。効いていないのは `ai-agents/` から `~/.claude` へ配っている資産だけ。

にもかかわらず `routines/prompts/*.md` は「rules が効かない前提」で書かれ、lint コマンド一覧・SKILL.md 規約・配布経路をベタ書きしている。repo の正と二重管理になり、実際にドリフトしている。ゴールは (1) プロンプトから重複を削って repo の正を参照させること、(2) `ai-agents/` をクラウドへ届ける筋を確定させること。

## Current structure

- `routines/*.json` の `prompt` は `routines/prompts/<name>.md` への薄いポインタ。プロンプト本文の変更はマージだけで次回実行に反映され、`/schedule` apply は不要（`routines/README.md`）。
- `.claude/rules/` は 8 本。すべて `paths:` frontmatter によるパススコープで、該当ファイルを触ったときだけロードされる。
- `ai-agents/scripts/verify-changed.sh` が変更ファイル種別 → lint/test を自動マッピングする決定論的検証スクリプトとして既に存在する（Go/Lua/Shell/Markdown/TOML/JSON/YAML/workflows/Dockerfile を網羅、ツールが無ければ `[SKIP]`）。人間向けエントリは `mise run verify:changed`。
- 一方でプロンプト側は同じ内容を手打ちしている。`weekly-adopted-issue-pr-bot.md` の手順 3-4 に lint コマンド表、`weekly-pr-care-bot.md` の手順 3-ii に同種の列挙、`weekly-adopted-issue-pr-bot.md:13` と `weekly-devx-skills-hooks-scan.md:20` に SKILL.md / hook の書式規約。
- ドリフト: `weekly-adopted-issue-pr-bot.md:15,17,36` と `weekly-devx-skills-hooks-scan.md:20,41` が `ai-agents/Makefile` を参照しているが、配布は mise タスクへ移行済みで Makefile は存在しない。
- `ai-agents/` の配布先はすべて `$HOME` 配下（`claude-link` / `skills-copy` / `agents-copy` / `settings-copy`）。クラウドには届かない。`ai-agents/settings/claude/settings.json` の hooks は `~/.claude/hooks/*.sh` というチルダ絶対パス参照で、仮にファイルを置いても解決しない。

## Design policy

**Phase 1（プロンプト整理）と Phase 2（plugin 化）は別 PR。** Phase 1 は JSON を触らないので apply 不要、マージだけで次回実行から反映される。

共通前置きファイル（`routines/prompts/_shared.md` のようなもの）は作らない。「repo の `.claude/**` は載る / `~/.claude/**` は載らない」は仕様として自明で、プロンプトに書き起こす価値がない。Phase 1 は重複の削除とドリフト修正に限定する。

`ai-agents/` をクラウドへ届ける手段は 3 つあるが、採るのは plugin 化。

1. repo の `.claude/` にコミット — 確実だが `ai-agents/` と実体が二重管理になる。
2. **plugin 化** — `.claude/settings.json` の `extraKnownMarketplaces` + `enabledPlugins` は cloud に carry over する。`ai-agents/` のレイアウト（`skills/<name>/SKILL.md`、`agents/*.md`）は plugin のレイアウトと既に一致しているため、追加するのはマニフェストだけで済む。**二重管理を作らずにローカルとクラウドの両方へ配れる唯一の筋。**
3. claude.ai 側でスキルを有効化 — skills のみ、repo 非依存。

setup script で `~/.claude` に配る迂回は採らない。hooks は「In a cloud session, Claude Code runs hooks from the repository and from your organization's server-managed settings」と明記されていて user-level settings が参照先に入っていないうえ、setup script は環境単位のスナップショットキャッシュ（初回のみ実行、約 7 日で失効）で repo の更新に追従しない。

Phase 2 は未確定事項が多いため PoC を先行させ、結果を設計記録に残してから本適用する。

## Implementation steps

### Phase 1-1: ベタ書きの重複を削る

1. `weekly-adopted-issue-pr-bot.md` の手順 3-4 の lint コマンド表を `ai-agents/scripts/verify-changed.sh` の実行 1 行に置換し、`[SKIP]` は「ツールが無くて未検証」であって PASS ではない旨を添える（SKIP が出たら PR body に書いて CI に委ねる）。
2. 同ファイル 13 行目の SKILL.md frontmatter 規約を削り、`.claude/rules/skill-authoring.md` へのポインタに圧縮する。
3. 同ファイル 15 / 17 / 36 行目の `ai-agents/Makefile`・「ルートに Makefile あり」を `mise.toml` のタスク（`mise run skills-copy` / `settings-copy` / `verify:changed`）へ修正する。
4. `weekly-pr-care-bot.md` の手順 3-ii、コンフリクト解消後の lint 列挙を同じく `verify-changed.sh` に置換する。
5. `weekly-devx-skills-hooks-scan.md` の 20 / 41 行目の `ai-agents/Makefile` を mise タスクへ修正し、skills / hooks の書式規約は `.claude/rules/skill-authoring.md` へのポインタに寄せる（判断軸のブログ URL とディレクトリ構成は残す）。
6. 残る scan 系 5 本（`daily-neovim-trend-scan` / `weekly-scripts-tooling-scan` / `weekly-environment-scan` / `weekly-ci-workflows-scan` / `monthly-routine-improve`）は Issue 起票が主で重複が無いため変更しない。

### Phase 1-2: ドキュメント追従

1. `routines/README.md` に「クラウド実行で効く設定 / 効かない設定」節を追記する（Background の表の要約）。運用者向けの記録であって、プロンプトには書かない。
2. `.claude/rules/routines.md` に「プロンプトに repo の正（`AGENTS.md` / `.claude/rules/` / `verify-changed.sh`）と重複する規約を書かない」を 1 行追加する。

### Phase 2-1: plugin PoC（hooks は含めない）

1. `ai-agents/.claude-plugin/plugin.json` を追加する（`name` / `version` / `description`。`skills` と `agents` は既定パスと一致するので指定不要）。
2. repo ルートに `.claude-plugin/marketplace.json` を追加する。plugin 1 件、`"source": "./ai-agents"`（相対パスは marketplace ルート = `.claude-plugin/` を含むディレクトリから解決される）。marketplace 名は予約語を避ける。
3. `.claude/settings.json` に `extraKnownMarketplaces`（`{"source": "github", "repo": "hodanov/my-pde"}`）と `enabledPlugins` を追加する。
4. `ai-agents/scripts/copy-entries.sh` は `ai-agents/skills` / `ai-agents/agents` を直接読むため、`.claude-plugin/` の追加による影響を受けない。cursor / codex / copilot 向けの配布は現状維持。

### Phase 2-2: クラウドでの実地検証

Routine を新規作成する必要はない。ドキュメントに「同じ環境が web / `claude --cloud` / routines すべてに適用される」とあるので、Routine と同じ環境 ID を使って `claude --cloud` でセッションを 1 本起動し、利用可能なスキル・サブエージェント・呼び出し名・plugin のインストール有無を報告させる。

### Phase 2-3: 設計記録と本適用

検証結果を `docs/plan/<日付>_ai-agents-plugin-distribution.md` に残したうえで、hooks の移植（`ai-agents/settings/claude/settings.json` の hooks ブロックを `ai-agents/hooks/hooks.json` へ切り出し、`~/.claude/hooks/*.sh` を `${CLAUDE_PLUGIN_ROOT}/settings/claude/hooks/*.sh` へ書き換える。macOS 通知系やローカル依存の hook は選別する）、mise 配布との併存方針、`version` bump 運用を決める。skills / subagents を Routine に実際に使わせるなら、最後に `routines/*.json` の `allowed_tools` へ `Skill` / `Task` を追加し `/schedule` で手動 apply する。

## File changes

| 変更 | パス                                                | 内容                                                                        |
| ---- | --------------------------------------------------- | --------------------------------------------------------------------------- |
| 編集 | `routines/prompts/weekly-adopted-issue-pr-bot.md`   | lint 表 → `verify-changed.sh`、SKILL 規約 → rules ポインタ、Makefile → mise |
| 編集 | `routines/prompts/weekly-pr-care-bot.md`            | lint 列挙 → `verify-changed.sh`                                             |
| 編集 | `routines/prompts/weekly-devx-skills-hooks-scan.md` | Makefile → mise、書式規約 → rules ポインタ                                  |
| 編集 | `routines/README.md`                                | 「クラウドで効く設定 / 効かない設定」節                                     |
| 編集 | `.claude/rules/routines.md`                         | 重複を書かない旨を 1 行                                                     |
| 新規 | `ai-agents/.claude-plugin/plugin.json`              | Phase 2: plugin マニフェスト                                                |
| 新規 | `.claude-plugin/marketplace.json`                   | Phase 2: marketplace カタログ                                               |
| 編集 | `.claude/settings.json`                             | Phase 2: `extraKnownMarketplaces` / `enabledPlugins`                        |

対象外: `AGENTS.md` の更新（`routines/` が Project Structure に無いギャップは `update-agents-md` 経由で別途）。`ai-agents/settings/claude/rules/` の 5 本を repo `.claude/rules/` へ複製すること（二重管理になる。lint コマンドは `verify-changed.sh` が担保する）。

## Risks and mitigations

- **プロンプトから削った知識が repo の正でカバーされていない**: 削除は 1 対 1 で突き合わせて確認する。とくに `~/.claude/rules/` にしかない汎用 lint 規約（Markdown / Shell / TOML / JSON-YAML）はクラウドに載らないが、コマンド自体は `verify-changed.sh` が担保する。
- **クラウドに lint ツールが無く `[SKIP]` が並ぶ**: SKIP は PASS ではない。プロンプトにその旨を明記し、PR body に残させて CI 側の判定に委ねる。
- **plugin 化で呼び出し名が名前空間化される**: プラグイン提供のサブエージェントは `plugin-name:agent-name` で参照される。skills も同様なら既存の `/commit-and-draft-pr` 等の呼び方が変わる。PoC で実際の表示名を確認してから本適用する。
- **plugin と mise 配布の二重ロード**: ローカルで `~/.claude/skills` のコピーと plugin の両方が載ると重複しうる。PoC 時にスキル一覧を確認し、必要なら Claude 向けの mise コピーを止める。
- **`version` の bump 忘れ**: `plugin.json` に `version` を書くと bump 時のみ更新が届く。クラウドは毎セッション install するため、古い版が固定されうる。運用ルールとして決める。
- **Routine が project-scope plugin の trust gate を通らない**: 通らなければ plugin 由来の資産はクラウドで載らない。PoC の主要な検証項目。

## Validation

Phase 1

1. `mise run verify:changed` — 変更した Markdown に markdownlint-cli2 と prettier が走る。
2. `git diff routines/` を読み、削った知識がすべて repo の正（`AGENTS.md` / `.claude/rules/*` / `verify-changed.sh`）でカバーされているか 1 対 1 で突き合わせる。
3. `grep -rn "Makefile" routines/prompts/` が 0 件になること。
4. マージ後、PR 作成系 Routine の次回定期実行のログで `verify-changed.sh` が実行され、`[PASS]` / `[SKIP]` が PR body に反映されているかを確認する。

Phase 2

1. ローカル: Claude Code を再起動し、`/plugin` で marketplace が登録され plugin が有効になっているか、スキル一覧に重複が出ていないかを確認する。
2. クラウド: `claude --cloud` セッションで plugin 由来のスキル / サブエージェントが見えるか、呼び出し名は何かを報告させる。
3. 上記が通ってから初めて `allowed_tools` を変更する。JSON を変えたら `/schedule` で apply し、宛先の `trigger_id` を間違えないこと。

## Open questions

- プラグイン提供の skill の呼び出し名は名前空間化されるのか。されるなら `ai-agents/agents.xml` や各 SKILL.md 内の相互参照（「`dev-workflow` を使え」等）の書き換えが必要になる。
- クラウドの Routine セッションは project-scope plugin の trust gate を通るのか。
- plugin hooks はクラウドで発火するのか。発火するなら `lint-changed.sh` や formatter 系をクラウドにも効かせられる。
- Claude 向けの mise 配布（`claude-skills-copy` / `claude-agents-copy` / `claude-settings-copy`）を plugin に一本化するか、併存させるか。cursor / codex / copilot 向けは plugin をサポートしないため mise 配布を残す必要がある。
