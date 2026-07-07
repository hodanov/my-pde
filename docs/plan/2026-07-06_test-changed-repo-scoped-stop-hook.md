# Plan: test-changed Stop hook（issue #509）— グローバル多重化をやめ、リポ限定 hook に再スコープする

Plan subagent（worktree isolation 試走を兼ねる）による調査・立案を整形したもの。

## Background

issue #509 は、`lint-changed.sh`（Stop, report-only）のテスト版 counterpart として、言語横断多重化（Go / npm / pytest）の
`test-changed.sh` を `ai-agents/settings/claude/hooks/` に追加し、`~/.claude/` へ配布してユーザーが触る全リポで走らせる提案。
「lint は決定論・テストは示唆どまり」という検証規律の非対称を埋めるのが動機。

ユーザーの評価軸は「テスト実行の決定論化はリポごとに実態が違いすぎて、グローバル多重化は費用対効果が悪い。
**このリポ限定ならアリ**」。調査の結果、この立場はリポジトリ自身の設計記録によって強く裏づけられており、
**リポ限定 hook（案 b）を推奨**する。狙う成果: my-pde 内では「変更した Go アプリのテストがターン終了時に必ず走り、
失敗が report-only で可視化される」状態を、配布物（`ai-agents/`）に一切触れずに実現する。

## Current structure

調査で判明した事実（すべて issue の前提評価に直結する）:

1. **`ai-agents/settings/claude/hooks/lint-changed.sh`**（Stop, exit 0 固定）: unstaged + staged + untracked の union を取り、
   拡張子で stylua / markdownlint / shellcheck / golangci-lint に多重化。**lint の多重化が成立するのは、各ツールが
   ファイル単位・秒未満・副作用なし・成否が決定的だから**。テストは entrypoint がリポ規約依存（mise / npm / pytest / make…）、
   実行時間非有界、副作用・ネットワークあり、flaky があり得る — 同じ構図は成立しない。
2. **リポ直下 `.claude/`** は配布対象外のリポ専用領域（`settings-copy` がコピーするのは `ai-agents/settings/claude` → `~/.claude` のみ）。
   しかも**リポ専用 hook の前例が既にある**: `.claude/settings.json` は PreToolUse で
   `$CLAUDE_PROJECT_DIR/.claude/hooks/guard-version-pins.sh` を配線済み。Claude Code はプロジェクト settings と
   ユーザー settings の hooks をマージして両方実行するので、`.claude/settings.json` に Stop を足せば
   グローバル Stop 群（lint-changed → skill-observe-nudge → notify-macos）と並走する。
3. **決定的な発見**: `ai-agents/scripts/verify-changed.sh`（+ mise `verify:changed`、verify-runner subagent の実行体）が、
   **issue #509 が作ろうとしている「変更ファイル→テスト」マッピングをこのリポについて既に実装している**
   （`scripts/*/(*.go|go.mod|go.sum|testdata/*)` → `mise run <app>:test` + lint、[PASS]/[FAIL]/[SKIP] 報告、fail で exit 1）。
   欠けているのは「Stop で自動発火する report-only の入り口」だけ。
4. **設計上の前例（最重要）**: `docs/plan/2026-07-04_verify-runner-agent.md` の Design policy は
   「**スクリプトは my-pde 専用、汎用性は verify-runner の LLM フォールバックで担う**。当初 repo 非依存スクリプト +
   symlink 配布で実装したが、bash の列挙的分岐が肥大化するため 2026-07-05 に簡素化した」と明記。
   **issue #509 のグローバル多重化は、このリポが一度作って捨てた方向への逆行**。初見リポのテストコマンド推論は
   verify-runner のフォールバック（`mise tasks ls` / `package.json` / `pyproject.toml` を見る）が既に担っている。
5. このリポのテスト実態は **Go のみ**（`scripts/` 配下 5 モジュール: ai-bridge / nvim-sync / config-diff / go-verify / scaffold、
   各 `<app>:test` = `go test ./...`）。AGENTS.md も「No repository-level test suite beyond per-directory checks」。
   `scripts/go-verify` は CI 同等の全モジュール一括ゲート（goimports + golangci-lint + `go test -count=1`）で、
   `-count=1` がテストキャッシュを無効化するため Stop ごとの実行には不向き。
6. hooks の出力規約: stdout はモデルへのコンテキスト、stderr はユーザー向け（`docs/plan/2026-04-05_hooks-output-visibility.md`）。
   lint-changed は stdout + exit 0 で報告する。

## Design policy

### 3 案の比較

**(a) issue 原案 — グローバル多重化 hook（`ai-agents/settings/claude/hooks/`）**: 不採用。

- verify-runner plan が明文で退けた「repo 非依存 bash 多重化」の再発明（Current structure 4）。汎用リポのテスト実行は
  verify-runner のフォールバックが既に担当しており、決定論が価値を持つ範囲（マッピング既知 = my-pde）を越える。
- 全リポの毎 Stop で走るコストが非有界: `npm test` は分単位の suite や watch モード（vitest 等）でハングし得る、
  `go test -short` は testing.Short() を実装したリポでしか効かない、sandbox/ネットワーク制約下の統合テストは
  恒常的に偽 FAIL を吐きノイズ化する。
- 原案の pytest 分岐は「変更されたテストファイルのみ実行」なので、プロダクションコードだけ変えた場合に何も走らない —
  保証したいはずの回帰検出がそもそも成立していない。lint と違いテストは「変更ファイル→検査対象」の写像が
  言語規約に依存し、多重化の各分岐が個別に穴を持つ。

**(c) 折衷 — リポ内規約スクリプトへ委譲する薄いグローバルディスパッチャ**: 不採用。

- 「リポごとに Stop の振る舞いを差し替える」機構は Claude Code がネイティブに持っている
  （プロジェクト `.claude/settings.json` の hooks マージ）。グローバルディスパッチャはプラットフォーム機能の再実装であり、
  委譲先の規約（スクリプト名・置き場）という新しい取り決めを全リポに増やすだけで、実利は (b) と同一。
  委譲先を持つリポが現状 my-pde 1 つしかない以上、(b) の上位互換にならない。

**(b) リポ限定 hook（推奨）**: `.claude/hooks/test-changed.sh`（新規、配布対象外）を `.claude/settings.json` の Stop に配線。
対象はこのリポの実態どおり **Go のみ**: 変更ファイルのうち `scripts/<app>/**`（\*.go / go.mod / go.sum / testdata）に
触れたアプリを dedup し、各アプリで `go test -timeout=60s ./...` を実行、失敗のみ stdout に報告して常に exit 0。

### 推奨案の設計判断

- **standalone hook にする（verify-changed.sh のラップはしない）**: verify-changed.sh を Stop から呼ぶと lint が二重実行になる
  （グローバル lint-changed と重複）うえ、report-only 化のために「fail で exit 1」という verify-runner が依存する契約へ
  モード追加が要る。`scripts/<app>` → `go test` の写像は case 1 個分で、AGENTS.md の規約（全 Go アプリが `<app>:test` を持つ）に
  固定されているため重複コストは小さい。verify-changed.sh と同じ path パターンを使い、乖離を防ぐコメントを相互に置く。
- lint-changed の regular idiom を踏襲: mise shims の PATH export、変更検出の 3 コマンド union、`command -v go` ガード、
  `TEST_CHANGED_DISABLE=1` オプトアウト、失敗時のみ stdout、常に exit 0。stdin は読まない
  （ブロックしないので loop guard 不要。`stop_hook_active` チェックは任意の最適化 — Open questions 参照）。
- **配布・Makefile・cursor/copilot への影響ゼロ**。`ai-agents/` に触れないので `settings-copy` も不要。
  worktree でも `.claude/` はバージョン管理されているため並列エージェント（#508 の文脈）でもそのまま効く。
- 実行コスト見積: 変更が Go 以外（nvim/lua、docs、ai-agents — このリポの大半のターン）なら早期 exit でほぼゼロ。
  Go 変更時も変更アプリのみ・ビルドキャッシュ有効で数秒/アプリ。

## Implementation steps

1. `.claude/hooks/test-changed.sh` を新規作成（下記仕様）。shfmt / shellcheck を通す。
2. `.claude/settings.json` に Stop 配線を追加:

   ```json
   "Stop": [{ "hooks": [{ "type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/test-changed.sh" }] }]
   ```

   （guard-version-pins と同じ `$CLAUDE_PROJECT_DIR` 形式。グローバル Stop とマージされ並走する。）

3. `ai-agents/scripts/verify-changed.sh` の Go マッピング箇所と新 hook の双方に、写像を共有している旨の
   相互参照コメントを 1 行ずつ追加（乖離検知用）。
4. `AGENTS.md` の Testing & Linting 節に 1 行追記（例: 「Claude では repo-local Stop hook `test-changed.sh` が
   変更 Go アプリのテストを report-only で自動実行する」）。dev-workflow スキルはグローバル資産なので
   リポ固有 hook には言及しない。
5. issue #509 に再スコープコメントを投稿（Validation 後、実装 PR で `Closes #509`）。

hook 仕様（骨子）:

```text
PATH に mise shims を前置 / set -u
TEST_CHANGED_DISABLE=1 なら exit 0
git repo 外なら exit 0
files = unstaged ∪ staged ∪ untracked（lint-changed と同一）
files の各行を case で scripts/*/(*.go|go.mod|go.sum|testdata/*) に照合し <app> を dedup 収集
apps が空 or go が無ければ exit 0
各 app: (cd scripts/<app> && go test -timeout=60s ./...) 失敗なら "- [go test] scripts/<app>" を蓄積
失敗があれば "test failures on changed Go apps (fix before marking the task done):" + 一覧を stdout
常に exit 0
```

### issue #509 への対応

issue #508 と同じ「前提検証 → 再スコープ」の記録をコメントで残す。要点: (1) グローバル多重化は verify-runner plan
（2026-07-05 の簡素化）で退けた方向の再発明で、汎用リポは verify-runner のフォールバックが担当済み、
(2) 折衷ディスパッチャは Claude Code のプロジェクト settings マージで代替可能なため不要、(3) 本リポのテスト実態は
Go のみなので、リポ専用 `.claude/hooks/test-changed.sh` として採用し「lint 決定論・テスト示唆」の非対称は
my-pde 内で解消する — として実装 PR でクローズ。

## File changes

- `.claude/hooks/test-changed.sh` — 新規。Go 専用・report-only の Stop hook（約 40 行）。
- `.claude/settings.json` — 編集。`hooks.Stop` を追加。
- `ai-agents/scripts/verify-changed.sh` — 編集（コメント 1 行のみ、挙動不変）。
- `AGENTS.md` — 編集（Testing & Linting に 1 行）。
- `docs/plan/2026-07-06_test-changed-repo-scoped-stop-hook.md` — 新規（本書）。

## Risks and mitigations

- **Stop ごとの実行コスト / go test のビルド時間**: 変更アプリのみ・キャッシュ有効で数秒。コールドキャッシュ初回や
  依存更新直後は遅くなり得るが `-timeout=60s` で頭打ち（コンパイル時間自体は非有界だが、5 モジュールとも小規模 CLI で
  実測上問題になりにくい）。macOS に GNU `timeout` が無いため外側のタイムアウトは張らない — 問題化したら再検討。
- **flaky / ノイズ**: report-only・exit 0 なので開発は止まらない。うるさければ `TEST_CHANGED_DISABLE=1` で即オフ。
- **マッピングの乖離**: verify-changed.sh と path パターンを重複保持する。相互参照コメント + 規約
  （AGENTS.md「全 Go アプリは `<app>:test` を持つ」）で drift を抑える。新アプリは `scripts/*` glob で自動追随。
- **偽陰性**: `scripts/<app>` 外の変更（`.golangci.yml` 等）ではテストが走らないが、これはスコープ限定の意図どおり。
  フル検証は verify-runner / `mise run verify:changed` / go-verify が担う既存の役割分担を変えない。
- **sandbox/ネットワーク**: 5 モジュールのテストはユニットテストでネットワーク不要。hook はサンドボックス外で走るため
  直近の sandbox 設定変更（#516）の影響も受けない想定。

## Validation

1. 単体: `scripts/scaffold` の Go ファイルを一時的にダーティにして hook を手動実行（`bash .claude/hooks/test-changed.sh`）
   → 何も出ず exit 0。テストを意図的に壊す → 失敗一覧が stdout に出て exit 0。非 Go 変更のみ / クリーンツリー /
   `TEST_CHANGED_DISABLE=1` → 即 exit 0。
2. lint: shellcheck / shfmt が hook を pass、`.claude/settings.json` は prettier、plan doc は markdownlint-cli2。
3. 結合: 新セッションで Go ファイルを変更してターンを終え、Stop で lint-changed → test-changed が並走し、
   テスト失敗時のみ報告が現れることを transcript で確認。
4. `mise run verify:changed` の挙動が不変であること（コメント追加のみ）を確認。

## Open questions

- 失敗報告に go test の要約 1〜2 行（失敗テスト名）まで含めるか、lint-changed と同じ「アプリ名のみ」に留めるか
  （初版はアプリ名のみを推奨。詳細は verify-runner に委譲）。
- `stop_hook_active` での早期 exit（継続ターンの再実行スキップ）を初版に入れるか。入れると stdin 読取が必要になり
  lint-changed とのイディオム差が生じる — 初版では見送り、コストが観測されたら追加で良い。
- 将来、他リポでも同じ需要が出た場合の指針を汎用文書（dev-workflow Notes や agents.xml）に
  「テストの Stop 自動実行はプロジェクト `.claude/settings.json` で行う」として規約化するか —
  本件では見送り、2 例目が出たときに issue 化。
