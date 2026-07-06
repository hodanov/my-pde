# Plan: WorktreeCreate hook による並行 AI エージェント作業の worktree 規約固定（issue #508 の再スコープ）

issue #508 は `scripts/worktree/` への Go 製 worktree マネージャ追加を提案しているが、
「AI エージェントの並列作業に効くか」という評価軸で検証した結果、Go ツールは作らず、
Claude Code の `WorktreeCreate` / `WorktreeRemove` hook と規約の文書化に再スコープする。

## Background

- **#508 の前提の半分は成立しない。** issue は「自律パイプラインが `auto/issue-NNN-*` ブランチを量産しており、
  複数エージェントが同一作業ツリーを取り合う」ことを動機とするが、これらのブランチを作っているのは
  `routines/prompts/weekly-adopted-issue-pr-bot.md` のクラウドルーチンで、毎回隔離されたクラウド環境で走る。
  ローカルのチェックアウトに contention は発生していない。isolation が実際に必要なのは、
  `docs/plan/2026-05-13_parallel-ai-agent-development.md` Phase 2 が想定するローカル複数セッション並走のみ。
- **Go ツールの付加価値は既存機能とほぼ重複する。** `rm` の未コミットガードは `git worktree remove` が標準で持つ
  （dirty なら `--force` 必須）。`new` の衝突時失敗も `git worktree add` の標準挙動。`origin/main` 基点も
  Claude Code の worktree 既定（`origin/HEAD` 起点）と同じ。`list` の dirty / ahead-behind 集計だけのために
  Go モジュール + CI 配線 + テスト一式を抱えるのは割に合わない。
- **残る本質的な要件は「配置規約の固定」だけ。** worktree を `~/workspace/.worktrees/<repo>/<slug>/` に置く規約を
  1 箇所に固定し、エージェントが規約を思い出さなくても守られる状態にすること。これは
  Claude Code の `WorktreeCreate` hook（worktree 作成ロジックを丸ごと差し替え、stdout で実パスを返す）で満たせる。
  hook にすれば `claude --worktree` / セッション中の `EnterWorktree` / subagent の `isolation: worktree` が
  すべて同じ規約に従い、クリーンアップ（クリーンなら自動削除、dirty なら保護）も本体側の仕組みに乗れる。
- subagent 案も不採用。worktree のライフサイクルは推論タスクではなくツール操作であり、
  セッション内並列は既存 subagent への `isolation: worktree` 指定で足りる。

## Current structure

- worktree 運用は未整備。リポジトリ全域に `worktree` への言及なし（#508 の指摘どおり）。
- hook の配線: `ai-agents/settings/claude/settings.json` の `hooks.*` に `~/.claude/hooks/*.sh` を登録し、
  実体は `ai-agents/settings/claude/hooks/*.sh`。`deploy-ai-config`（`ai-agents/Makefile`）で `~/.claude/` へ配布。
- 既存 hook の流儀: `#!/usr/bin/env bash` + mise shims の PATH 補完 + `set -u`、shellcheck / shfmt 対象。
- worktree 置き場の規約は `docs/plan/2026-05-13_parallel-ai-agent-development.md` の Decisions で
  `~/workspace/.worktrees/{REPOSITORY}/<branch-name>/` と確定済み（同プラン Phase 2 には
  「小スクリプトを scripts/ に追加」とあり、本プランはこれを hook 方式に置き換える）。

## Design policy

- **バイナリではなく hook で規約を固定する。** 規約は `worktree-create.sh` 1 本に閉じ、
  Claude Code の全 worktree 作成経路（`--worktree` / `EnterWorktree` / subagent isolation）に効かせる。
- **git 標準のガードを再実装しない。** 衝突時失敗・dirty 保護・cleanup は git と Claude Code 本体に任せる。
- **repo 非依存に書く。** repo 名は hook が受け取る `source_path` から導出し、どのリポジトリでも
  `~/workspace/.worktrees/<repo>/<slug>/` に切れるようにする（ai-agents の repo 非依存方針に合致）。
  置き場は `WORKTREE_BASE` 環境変数で上書き可能にする（既定 `~/workspace/.worktrees`）。
- **他 CLI（cursor / codex / copilot）と手動運用はフォールバックで足りる。** `--worktree` 相当がない CLI 向けには
  AGENTS.md に素の `git worktree` 手順と規約を明文化する。手動では素の git worktree で十分という
  運用実感があるため、専用 skill の新設は観察してから判断する（YAGNI）。
- **`list` 相当は作らない。** 必要になったら `git worktree list` を叩けばよい。定型化したくなったら
  その時に alias / skill を検討する。

## Implementation steps

1. **`worktree-create.sh` hook を追加**（`ai-agents/settings/claude/hooks/`）
   - stdin JSON: `name`（worktree 名）と `cwd`（起動ディレクトリ）を `jq` で読む
     （スキーマはライブ実行でキャプチャして確認済み。公式ドキュメント要約の
     `worktree_path` / `source_path` は実際には来ない）。
   - リポジトリは `git -C "$cwd" rev-parse --show-toplevel` で導出（サブディレクトリ起動にも対応、
     リポジトリ外なら失敗）。`dest="${WORKTREE_BASE:-$HOME/workspace/.worktrees}/$repo/$name"`。
   - 既定挙動を踏襲して `origin/HEAD` 基点でブランチを切り `git worktree add`
     （fetch 失敗時はローカル `HEAD` にフォールバック）。ブランチ名は仮置きで `<name>` と同名にする。
     hook はセッション開始前に走るため、この時点で AI に命名させることはできない。
     正式なブランチ名 `<slug>` はセッション内で AI が命名してリネームする（step 4 の規約）。
   - 成功時のみ `dest` を stdout に出力。既存パス / 既存ブランチとの衝突は `git worktree add` の失敗を
     そのまま非ゼロ終了で伝播（上書きしない）。
2. **settings に配線**: `ai-agents/settings/claude/settings.json` の `hooks.WorktreeCreate` に
   `~/.claude/hooks/worktree-create.sh` を登録。
3. **配布**: `deploy-ai-config` で `~/.claude/` へ反映。
4. **AGENTS.md に運用規約を追記**: 配置規約（`~/workspace/.worktrees/<repo>/<name>/`。`<name>` は
   `--worktree` の引数 or 自動生成名で、使い捨てで良い）、ブランチ命名規約
   （作業内容が固まったら AI が prefix なしの `<slug>` を命名し `git branch -m <slug>` でリネームする。
   checkout 中でもリネーム可能）、他 CLI / 手動では素の `git worktree add` を規約パスに対して使うこと、
   撤去は `git worktree remove`（dirty 保護は git 標準）で行うこと。
   あわせて `commit-and-draft-pr` スキルに「push 前にブランチ名が仮名（worktree ディレクトリ名と同名や
   自動生成名）のままなら、作業内容から `<slug>` を命名してリネームしてから push する」ガードを追記する。
5. **試走で挙動確認**（Validation 参照）: 規約パスに作成されるか、subagent isolation でも hook が効くか、
   セッション終了時の cleanup が hook 作成の worktree にも働くか。働かない場合のみ
   `WorktreeRemove` hook（`git worktree remove` + `prune` の側効果実行）を追加する。
6. **記録の更新**: #508 に再スコープの理由をコメント（前提検証の結果と本プランへのリンク）。
   `docs/plan/2026-05-13_parallel-ai-agent-development.md` の Decisions に
   「worktree 補助は scripts/ のスクリプトではなく WorktreeCreate hook で実現」を追記。

## File changes

| File                                                    | Change                                                               |
| ------------------------------------------------------- | -------------------------------------------------------------------- |
| `ai-agents/settings/claude/hooks/worktree-create.sh`    | 新規。配置規約を実装する WorktreeCreate hook                         |
| `ai-agents/settings/claude/settings.json`               | `hooks.WorktreeCreate` の配線を追加                                  |
| `AGENTS.md`                                             | worktree 運用規約（配置・命名・他 CLI 向けフォールバック手順）を追記 |
| `ai-agents/skills/commit-and-draft-pr/SKILL.md`         | push 前の仮ブランチ名検出 → `<slug>` へのリネームガードを追記        |
| `docs/plan/2026-05-13_parallel-ai-agent-development.md` | Decisions に hook 方式への置き換えを追記                             |
| `ai-agents/settings/claude/hooks/worktree-remove.sh`    | 条件付き新規。試走で cleanup が効かない場合のみ追加                  |

## Risks and mitigations

| Risk                                                                       | Mitigation                                                                                          |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| hook 化により `.worktreeinclude`（gitignore 済みファイルのコピー）が無効化 | このリポには `.env` 等のコピー対象がない。必要になったら hook 内でコピー処理を足す                  |
| hook 作成の worktree はセッション transcript の relocation 対象外          | 影響は `--resume` 時の探し先のみで軽微。AGENTS.md に注記                                            |
| 本体の自動 cleanup が hook 作成分に働かない可能性                          | 試走で確認し、働かなければ `WorktreeRemove` hook を追加。最終手段は手動 `git worktree remove`       |
| claude 以外の CLI には hook が効かず規約が自己申告になる                   | AGENTS.md への明文化でカバー。逸脱が観察されたら skill 化を再検討                                   |
| hook のバグで worktree 作成が全滅する（非ゼロ終了は作成失敗になる）        | スクリプトは最小限に保ち、shellcheck / shfmt / lint-changed.sh の既存検証に乗せる。試走を必須とする |
| AI がブランチのリネームを忘れて仮名のまま push する                        | `commit-and-draft-pr` の push 前ガードで捕捉。漏れても `git branch -m` + push し直しで回復可能      |

## Validation

- [ ] `claude --worktree test-name` で `~/workspace/.worktrees/my-pde/test-name/` に worktree が作成される
- [ ] my-pde 以外のリポジトリでも同じ規約パスに作成される（repo 非依存の確認）
- [ ] 既存の worktree 名と衝突した場合に上書きせず失敗する
- [ ] セッション内で AI が `git branch -m <slug>` によりタスク内容に即したブランチ名へリネームできる
      （commit-and-draft-pr の push 前ガードが仮名を検出する）
- [ ] subagent（`isolation: worktree`）でも hook 経由で規約パスに作成される
- [ ] セッション終了時、クリーンな worktree が片付くこと（片付かない場合は WorktreeRemove hook を追加して再確認）
- [ ] shellcheck / shfmt が pass し、settings.json が jq でパース可能
- [ ] AGENTS.md の規約記述と #508 コメント・2026-05-13 プランの Decisions 更新が揃っている

## Decisions

会話で確定済みの方針を記録（Open questions から昇格）。

- **ブランチ名に prefix は付けない**: `wt/` や `agent/` は不要。プレーンな `<slug>` とする
  （worktree 上のブランチかどうかを名前で区別する必要はない）。
- **worktree ディレクトリ名とブランチ名は別物にする**: `--worktree xxxx` の引数（or 自動生成名）は
  作業に入る時点でちゃんとした命名ができないケースが圧倒的に多いため、使い捨てで良い。
  正式なブランチ名 `<slug>` は、セッション内でタスクを理解した AI が命名し、
  hook が切った仮ブランチを `git branch -m <slug>` でリネームする
  （hook はセッション開始前に走るため、作成時点での AI 命名は構造上不可能）。

## Open questions

- cursor / codex / copilot 側で並列運用の頻度が上がった場合に、フォールバック手順を skill に昇格するか。
