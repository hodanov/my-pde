# Plan: 並列AIエージェント開発のための環境整備

複数のClaude Codeエージェントを同時並行で走らせるための基盤を、本リポジトリ（my-pde）上に段階的に整える。Boris氏のような10並列規模を最終目標としつつ、まずは2並列から始めてボトルネックを観察し、隔離・自律性・仕様・レビューの4層で必要な仕掛けを足していく。

## Background

- 単一エージェントへの逐次指示では開発速度に限界があり、待ち時間が累積する
- Claude Codeの開発チームは10以上の並列タスクを回す運用をしており、これは単なる多重起動ではなく、ワークフロー全体の設計シフト
- 本リポジトリには既に `ai-agents/skills/`（17スキル）、`ai-agents/settings/claude/hooks/`（6個のフォーマッタhook）、`commit-and-draft-pr` スキル等の素地が揃っている
- ユーザーの役割を「実装者」から「設計者・レビュアー」に移行させることが本質的なゴール

## Current structure

- `.claude/settings.local.json`: 個人用permission allowlist。`Bash(go:*)`, `Bash(make:*)` 等が許可済み
- `.claude/settings.json`: 未作成（プロジェクト共有のsettingsはここに置く想定）
- `ai-agents/settings/claude/hooks/`: `goimports.sh`, `prettier.sh`, `stylua.sh`, `shfmt.sh`, `markdown-format.sh`, `tombi.sh`
- `ai-agents/skills/`: `commit-and-draft-pr`, `review`, `review-scan`, `investigate` などレビュー・コミット系スキル
- `AGENTS.md`: プロジェクト全体の開発ガイド（lint/test表あり）
- worktree運用は未整備（同一クローン上でブランチ切替のみ）

## Design policy

- **段階導入**: いきなり10並列を目指さず、2並列で1〜2週間運用してボトルネックを観察してから次の整備に進む
- **既存資産の再利用**: 新しいhookやスキルを作る前に、既存の `ai-agents/settings/claude/hooks/` と `ai-agents/skills/` を最大限活かす
- **疎結合タスクのみ並列化**: 同一ファイルを触る作業は直列化前提とし、並列化対象は領域が分離しているもの（例: `dotfiles/` と `nvim/` と `scripts/ai-bridge/`）に限定する
- **draft PRで出口を統一**: エージェントは必ず draft PR で終了し、マージ判断は人間に残す
- **失敗を恐れず観察優先**: 並列化で問題が出たらその都度hookやスキルで対処する。先回りした作り込みはしない
- **起動はClaude Code複数ウィンドウ**: 並列エージェントの起動方式は Claude Code を複数ウィンドウで開く方式に統一する。subagentやscheduleは併用しない（最初は）
- **worktreeはリポ外に置く**: worktreeは `~/workspace/.worktrees/{REPOSITORY}/` 配下に集約する。リポ内に置くとAIエージェントへ食わせる際のノイズになり、git管理上もハンドリングが面倒になるため
- **自動レビューは内製スキルのみ**: `review` / `review-scan` スキルで運用する。`ultrareview` は別途費用が発生するため採用しない

## Implementation steps

### Phase 1: 基盤整備（最初の1週間）

1. **権限の棚卸しと整理**
   - `fewer-permission-prompts` スキルを実行し、直近の transcripts から頻出する許可項目を抽出
   - `.claude/settings.local.json` の個人用エントリのうち、プロジェクト共有すべきものを `.claude/settings.json` に昇格させる
   - `Bash(rm:*)`, `Bash(git push --force:*)` 等は明示的に `deny` へ
2. **PostToolUse hook の有効化**
   - 既存の `ai-agents/settings/claude/hooks/*.sh` を `.claude/settings.json` の `hooks.PostToolUse` に登録
   - Edit/Write後に対応するフォーマッタ＋lintが自動で走る状態にする
3. **タスクブリーフのテンプレート整備**
   - `docs/plan/` 配下に並列タスク用のブリーフテンプレートを1枚追加
   - 含める項目: ゴール / 触っていいファイル範囲 / 触ってはいけないファイル / 完了条件 / テストコマンド / draft PR タイトル規約

### Phase 2: 隔離環境（次の1週間）

1. **git worktree 運用の確立**
   - worktree置き場は `~/workspace/.worktrees/{REPOSITORY}/<branch-name>/` に統一
     - リポジトリの外に置くことでリポ内ノイズを排除し、AIエージェントに食わせるコンテキストをクリーンに保つ
     - `{REPOSITORY}` ごとにディレクトリを分けることでブランチ名衝突を回避
   - worktree作成・破棄を自動化する小スクリプトを `scripts/` に追加（`make worktree-new BRANCH=feat/foo` 等）
     - 内部で `git worktree add ~/workspace/.worktrees/{REPOSITORY}/<branch> <branch>` を実行
     - 不要になったら `make worktree-remove BRANCH=...` で破棄
   - ブランチ命名規則を `agent/<scope>/<short-desc>` で統一
2. **ポート・環境変数衝突の回避**
   - dev server を立てるタスクはworktreeごとに `PORT` を変えるルールを明文化（AGENTS.md追記）
3. **2並列の試走（Claude Code複数ウィンドウ）**
   - 領域が完全に分離した2タスクを選び、それぞれ別worktreeで Claude Code を別ウィンドウで起動
   - 詰まりポイントをログに残す（`docs/log/`）

### Phase 3: レビュー流量制御（2週間目以降）

1. **commit-and-draft-pr スキルの並列対応確認**
   - 並列実行時にPRタイトル・ブランチがバッティングしないことを確認
2. **CIの整備状況確認**
   - `.github/workflows/` の9ワークフローが draft PR でも回ることを確認
   - draft でも必須にしたいチェックがあれば設定追加
3. **自動レビューの導入（`review` / `review-scan` 運用）**
   - `review-scan` スキルで一次フィルタ（Phase.1: 広く浅くスキャン）
   - 優先度が高い指摘について `review` スキルで深掘り（Phase.2）
   - `ultrareview` は費用が発生するため不採用

### Phase 4: 並列度の段階拡大

1. 2並列 → 4並列 → ボトルネック観察 → 対応 → 拡大、を繰り返す
2. 観察結果は `docs/log/` に蓄積し、必要なら新規スキル/hookに昇華

## File changes

| File                                        | Change                                                              |
| ------------------------------------------- | ------------------------------------------------------------------- |
| `.claude/settings.json`                     | 新規作成。共有 allow/deny と PostToolUse hooks を登録               |
| `.claude/settings.local.json`               | プロジェクト共有すべき項目を `settings.json` に移し、個人用のみ残す |
| `docs/plan/parallel-task-brief-template.md` | 並列タスク用ブリーフテンプレート（Phase 1 で追加）                  |
| `AGENTS.md`                                 | worktree運用ルール・ポート割当ルールの追記（Phase 2）               |
| `scripts/worktree-new.sh` or `Makefile`     | worktree作成・破棄の補助スクリプト（Phase 2）                       |
| `docs/log/YYYY-MM-DD_parallel-trial-*.md`   | 各試走の観察ログ                                                    |

## Risks and mitigations

| Risk                                                    | Mitigation                                                                      |
| ------------------------------------------------------- | ------------------------------------------------------------------------------- |
| permission allowlist が広すぎて危険操作が通る           | `deny` を先に厳しく書く。`rm`, `force push`, DB 破壊系を明示的にブロック        |
| 同一ファイルへの並列編集でマージコンフリクト多発        | タスク分解時に「触ってよいファイル範囲」を明示。重なる作業は直列化              |
| worktreeごとの依存解決でディスク・時間コスト増          | pnpm / uv 等のコンテンツアドレッサブルなツールを優先。Docker volumeの共有も検討 |
| dev server / DB のポート衝突                            | worktreeごとに `PORT` を環境変数で変える運用を明文化                            |
| レビュー律速で並列度のメリットが消える                  | draft PR + `review-scan` / `review` スキルで一次フィルタを噛ませる              |
| エージェントがhookを抜けるために `--no-verify` 等を使う | hook失敗時の挙動を `deny` 寄りに調整。CLAUDE.md でも禁則明記                    |

## Validation

- [ ] `fewer-permission-prompts` 実行後、`.claude/settings.json` に共有allowlistが整理されている
- [ ] Edit/Write 後に対応するフォーマッタ hook が自動実行される（goimports / prettier / stylua 等）
- [ ] 危険操作（`rm -rf`, `git push --force`）が `deny` でブロックされる
- [ ] 並列タスク用ブリーフテンプレートが `docs/plan/` に存在する
- [ ] worktree を2つ並行作成・破棄できるスクリプトまたは Makefile ターゲットがある
- [ ] 2並列で疎結合タスクを走らせて、両方が draft PR まで到達する
- [ ] 試走の観察結果が `docs/log/` に記録されている
- [ ] CI が draft PR でも期待通り回る

## Decisions

会話で確定済みの方針を記録（Open questions から昇格）。

- **worktree置き場**: `~/workspace/.worktrees/{REPOSITORY}/<branch-name>/`
  - リポ内に置くとAIエージェントに食わせる際のノイズになり、git管理も煩雑になるため外置きにする
  - `{REPOSITORY}` ごとにディレクトリを分けてブランチ名衝突を回避
- **worktree補助の実現方式（2026-07-06 更新）**: Phase 2 の「小スクリプトを `scripts/` に追加」は
  Claude Code の `WorktreeCreate` hook で配置規約を固定する方式に置き換えた。ブランチ命名も
  `agent/<scope>/<short-desc>` ではなく、セッション内で AI が命名する prefix なしの `<slug>`
  （worktree ディレクトリ名とは別名）に変更。
  詳細は `docs/plan/2026-07-06_worktree-hooks-for-parallel-agents.md` を参照
- **起動方式**: Claude Code 複数ウィンドウ（subagent / schedule は併用しない）
- **自動レビュー**: `review` / `review-scan` スキルで運用。`ultrareview` は費用発生のため不採用
- **hook の負荷調整**: 開発体験が重くなってから対応する（先回りはしない）
- **並列度の上限**: 観察結果が出てから判断する（事前に決めない）

## Open questions

- （現時点で未確定の論点はなし。試走後の観察結果に応じて追記する）
