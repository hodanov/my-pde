# Plan: dev-workflow の explore フェーズに subagent 委譲ルールを追加する

`ai-agents/skills/dev-workflow/SKILL.md` を v6 → v7 に改訂し、verify フェーズだけにある subagent 委譲ルールを explore フェーズにも対称に置く。委譲先は組み込みの `Explore` に寄せ、「当たりを付けるまでは subagent、確定した対象は main で Read」という境界を発動条件付きで明文化する。

## Background

### コンテキスト単価の実測

`scripts/agent-stats` で `~/.claude/projects/**` の transcript を集計した（2026-08-28 実行、`--since 720h` = 直近 30 日、128 セッション / 9,256 assistant turns）。

- main loop: 7,411 turns、cache_read 153,325 / turn
- subagent: 1,845 turns、cache_read 52,152 / turn
- **main の 1 ターンあたりコンテキスト単価は subagent の 2.94 倍**
- subagent は全ターンの **19.9%** を占めながら cache_read は **7.8%** しか使っていない
- `Agent` 呼び出しは 171 回 / 128 セッション = **1.34 回/セッション**
- `Read` の内訳は main 1,649 / subagent 1,220。**探索の 57.5% がメインコンテキストで起きている**

全期間（132 セッション / 9,345 turns）でも同じ形になる: main 152,385 / turn、subagent 51,427 / turn（2.96 倍）、subagent ターン比 20.1%、cache_read 比 7.8%、`Agent` 174 回。

依頼元の独立した計測（本プランでは追試していない）では、`Agent` の tool_result が平均 1,493 chars（≒373 tokens）しかなく、「探索を Grep に置き換える」より「探索まるごとを別 context に出す」ほうが効果が 10〜18 倍高い、main ターンの 20% を subagent に移すと cache_read −12.6% という見積りが出ている。単価 2.94 倍という本プランでの実測はこの見積りと整合する。

### 穴の所在

SKILL.md は verify フェーズを `verify-runner` に委譲するルールを L64 に持つ。一方 explore フェーズには委譲ルールが無く、L47 / L55 は「Plan Mode で読む」= メインコンテキストで読む形になっている。**書いてあるルールと書いていないルールで、実際の行動が分かれている。**

observations 69 件（`ai-agents/skills/dev-workflow/observations/`）を数えた結果:

- Plan Mode / `EnterPlanMode` への言及: **50 件（72%）**
- `verify-runner` への言及: **40 件（58%）** ← SKILL.md に書いてある
- `Explore` agent への言及: **8 件（12%）** ← SKILL.md に書いていない
- `investigation-scout` / `investigation-diver` への言及: **0 件**

Plan Mode で explore している 50 件のうち、explore を subagent に出したと記録されているのは 8 件（16%）にとどまる。**書けば 58%、書かなければ 12%** という差が、この改訂の期待値そのものである。

### 一次証拠（observations）

委譲が効いた側:

- `2026-08-26_001`: 「Plan Mode 側で explore/plan が完結していたため、本スキル呼び出し後は『implement → verify → commit』だけに専念でき、**二重探索が発生しなかった**」
- `2026-08-07_001`: 「複数の general-purpose agent を並列起動して 9 本の API を手分けして調査させたことで、通常の explore より深く広く検証でき、齟齬の早期発見に繋がった。この『並列 agent による外部仕様の一次情報検証』は、外部 API 依存の実装タスクの explore パターンとして**再利用価値が高い**」
- `2026-08-19_001` / `2026-08-21_002` / `2026-08-14_002`: Plan Mode 中に Explore agent を 2〜3 並列で起動した実績（後述の「Plan Mode 中でも委譲は成立する」の根拠）

委譲のやりすぎが害になった側:

- `2026-08-24_002`: 「途中で『他 agent の完了を待つだけ』の目的で無意味な placeholder agent を 2 回誤って起動してしまった（実害はないが、**待機に Agent tool を使うべきでない**）」
- `2026-08-14_002`: verify-runner の 1 回目の要約が不十分で「SendMessage で該当エージェントに再度完全な結果報告を依頼し、…**追加のやりとりが 1 回発生した**」← 結論だけ返ることで判断材料が薄くなる実例
- `2026-08-07_002`: 委譲プロンプトに「サンドボックス無効化して再実行」を含めたため「auto mode の安全分類器にブロックされた（"Create Unsafe Agents" として拒否）」

非委譲が正しかった側の基準:

- `2026-08-17_002`: 「委譲の判断基準としては『変更が依存アップグレードや複数モジュールに及ぶなど**出力量が事前に読めない場合は委譲する**』という基準が実務上機能している」
- `2026-08-19_001`: 「変更ファイルが 2 つのみで出力量が少なく事前に読めたため…この基準は今回も一貫して機能した」

## Current structure

- `ai-agents/skills/dev-workflow/SKILL.md`（v6, 10.0KB）。`~/.claude/skills/dev-workflow/SKILL.md` と内容一致（デプロイ済み。v5 の未デプロイ事故は再発していない）
- 該当行: L26 適用範囲 / L47 new product の explore / L50 new product の verify / L55 existing product の explore / L59 existing product の verify / L62–74 `## 検証ルール（共通）`
- `ai-agents/agents/` に 10 定義（`investigation-scout` / `investigation-diver` / `code-review-*` / `review-*` / `verify-runner` / `textlint-fixer`）。`mise run agents-copy` で配布
- `ai-agents/skills/investigate/SKILL.md`（v1, `disable-model-invocation: true`）が scout → diver の 2 フェーズ調査パイプラインを持つ
- `ai-agents/agents.xml`（= `~/.claude/CLAUDE.md` の symlink 先）は「非自明な実装では dev-workflow を invoke せよ」という入口だけを持ち、フェーズ内の手順は skill 側に置く分業になっている

### 実際に使われている subagent_type（transcript 実測）

`grep -roh '"subagent_type": *"[^"]*"' ~/.claude/projects/` で全 167 件を集計した。

- `Explore` 97（58%）
- `verify-runner` 25
- `Plan` 15
- `fork` 12
- `general-purpose` 8
- `claude-code-guide` 4、`review-comments` 2、`review-go-test` / `review-correctness` / `review-code-design` / `code-reviewer` 各 1
- **`investigation-scout` 0、`investigation-diver` 0**

リポジトリが定義した 2 フェーズ調査エージェントは、167 回の `Agent` 呼び出しと 114 件の observations（dev-workflow 69 / commit-and-draft-pr 34 / permission-prompt-tuner 10 / update-agents-md 1）を通して **一度も使われていない**。

## Design policy

### 委譲先は `Explore` 単一に寄せる

「探索の広さで使い分ける」は採らない。理由は 3 つある。

第一に、実測で `Explore` が 97/167 と圧倒的に使われており、scout/diver は 0 回。使われていない選択肢を SKILL.md に書くと、v6 の references と同じ「書いたが読みに行かれない」状態を再生産する。第二に、使い分けルールそれ自体が判断コストになる。verify 側が `verify-runner` 単一入口で 58% の言及率を得ているのは、判断が要らないからである。第三に、scout → diver は Scout Report → Diver Report の 2 往復を前提とした調査専用の形式で、実装タスクの explore（「これから編集するファイルを決める」）には出力形式が合わない。

したがって:

- dev-workflow の explore の委譲先は **`Explore`**（組み込み）に一本化する
- 外部コマンド（`gh` / MCP / API spec ツール）を叩く調査だけ `general-purpose` に回す。根拠は `2026-08-07_001` の一次証拠
- 実装に入らない独立した原因調査は `investigate` スキルの領分とし、**1 行のポインタだけ** 置く。v6 が `dependency-update` に対して採った扱い方の踏襲で、手順は写さない

### 境界は「対象が特定済みか」で引く

全部委譲は不可能である。編集するファイルは main で Read しなければ Edit できず、プランに `file:line` を書くなら根拠を main で持っていなければならない。境界は「当たりを付けるまでは subagent、確定した対象は main で Read」に引く。

これは verify 側で既に機能している基準（`2026-08-17_002` / `2026-08-19_001` の「出力量が事前に読めない場合は委譲する」）の explore 側への横展開である。**新しい基準を発明せず、実務で検証済みの基準を対称化する**ことで、判断の一貫性を保つ。

同時に「要約だけを根拠に編集やプランを書かない」を明記する。`2026-08-14_002` の verify-runner で起きた「要約が薄くて往復が増えた」を explore で繰り返さないための歯止めであり、`investigation-diver` の "Evidence over intuition"（`ai-agents/agents/investigation-diver.md`）と同じ規律でもある。

### Plan Mode 中でも委譲は成立する（実測で確認済み）

「Plan Mode 中に subagent を起動できるのか」は実測で解決している。`2026-08-19_001`（3 並列 Explore agent）、`2026-08-21_002`（Explore agent 2 件・Plan agent 1 件を並行起動）、`2026-08-14_002`（3 並列 Explore サブエージェント）はいずれも Plan Mode 内での起動である。制約は無い。

プランの品質低下も観測されていない。むしろ `2026-08-26_001` は二重探索の消滅を成果として記録している。ただし SKILL.md の適用範囲節（L26）は「Plan Mode で済んでいれば explore/plan を重複させない」と書くだけで、**Plan Mode の内部でどう探索するか**には何も言っていない。ここが穴なので、1 文足して探索ルールが Plan Mode 側にも及ぶことを明示する。

### verify-runner の記述と同じ文型にする

対称性は文型レベルで取る。現行 L64 は「委譲先 → 委譲する理由 → 失敗時の扱い → 使えない環境での縮退」の 4 要素で書かれている。explore 側も同じ 4 要素で書く。読み手が verify の記述を既に内在化しているなら、同型の記述は追加の学習コストなしで効く。

### 「常に委譲」とは書かない

委譲のやりすぎには実測された害がある。待機目的の誤起動（`2026-08-24_002`）、要約が薄くて往復が増える（`2026-08-14_002`）、委譲プロンプトが安全分類器にブロックされる（`2026-08-07_002`）。したがって発動条件とアンチパターンを併記し、無条件の「常に委譲」は書かない。

### references には逃がさない

追記は 6 bullet 程度に収め、`references/` への切り出しはしない。v6 が新設した `references/verification-points.md` が実際に読みに行かれたかは次サイクルの評価待ちであり（v6 プランの「リスクと次サイクルの評価軸」に明記）、効果未確認の仕組みに同じ賭けを重ねない。

### version は 7 に上げ、amendment を起こす

`.claude/rules/skill-authoring.md` が "Bump `metadata.version` when amending a skill" を required としているため、6 → 7 は必須。加えて v6 プランが「以後 amendment の完了条件にデプロイと version 照合を含める」と決めているので、`observations/amendments/` に amendment を起こし、デプロイ後の version 照合まで完了条件に入れる。

本改訂は `skill-improve` の Observe → Inspect → Amend → Evaluate ループに **載せる**。理由: (1) 根拠の一次証拠が observations にある、(2) 変更が SKILL.md 本文への追記である、(3) 次サイクルの Evaluate が効果測定の受け皿になる。ただし `/skill-improve dev-workflow --apply` を通すのではなく、本プラン + amendment を手で起こす形にする。今回の起点は observations の再発パターン分析ではなく transcript のトークン実測であり、`skill-improve` の Step 3（4 観点のパターン分析）が主たる根拠ではないため。

## Implementation steps

1. `ai-agents/skills/dev-workflow/SKILL.md` を編集する（frontmatter の version、L26、L47、L55、新セクション追加の 5 箇所）。
2. `ai-agents/skills/dev-workflow/observations/amendments/2026-08-28_001_amendment.md` を `ai-agents/skills/skill-improve/template.md` のフォーマットで作成する。分析サマリに本プランの実測ベースラインを転記し、次サイクルの評価軸を書く。
3. リポジトリルートから変更ファイルにスコープした `markdownlint-cli2` と `prettier --check` を実行する（`mise run lint:format` は作業ツリー全体を舐めるため、無関係な未追跡ファイルに汚染される）。
4. `mise run skills-copy` で配布し、`~/.claude/skills/dev-workflow/SKILL.md` の `version: 7` を確認する。
5. 効果測定のベースラインを amendment に固定する（本プランの数値をそのまま採用）。
6. `commit-and-draft-pr` スキルでコミットする。`git commit` を単体で直接実行しない。

## File changes

### `ai-agents/skills/dev-workflow/SKILL.md`（編集, v6 → v7）

frontmatter:

```yaml
metadata:
  version: 7
```

**L26（適用範囲）** — 変更前:

```markdown
- explore / plan がハーネス側の仕組み（Claude Code の Plan Mode 等）で既に済んでいる場合、その2フェーズは重複させず、**implement 以降（テスト戦略 → verify → commit）から適用する**。本スキルの中核価値は検証とコミットの規律にある。
```

変更後（末尾に 1 文追加）:

```markdown
- explore / plan がハーネス側の仕組み（Claude Code の Plan Mode 等）で既に済んでいる場合、その2フェーズは重複させず、**implement 以降（テスト戦略 → verify → commit）から適用する**。本スキルの中核価値は検証とコミットの規律にある。ただし Plan Mode 側で explore する場合も「探索ルール（共通）」は適用する。探索がどのフェーズで行われても、当たりを付ける読み込みは隔離コンテキストで行う。
```

**L47（new product の explore）** — 変更前:

```markdown
1. **explore**: Plan Mode で既存コードと構造を読み、関連モジュール・依存・既存パターンを把握する。このフェーズではコードを書かない。
```

変更後:

```markdown
1. **explore**: Plan Mode で既存コードと構造を読み、関連モジュール・依存・既存パターンを把握する。読み込みは `Explore` サブエージェントへの委譲を推奨（下記「探索ルール（共通）」参照）。このフェーズではコードを書かない。
```

**L55（existing product の explore）** — 変更前:

```markdown
1. **explore**: 関連コード・git 履歴・既存テストを読み、現挙動を理解。問題の根本原因や変更範囲を特定する。コードは書かない。
```

変更後:

```markdown
1. **explore**: 関連コード・git 履歴・既存テストを読み、現挙動を理解。問題の根本原因や変更範囲を特定する。読み込みは `Explore` サブエージェントへの委譲を推奨（下記「探索ルール（共通）」参照）。コードは書かない。
```

**新セクション** — `## 既存プロダクト改修（existing product）` の直後、`## 検証ルール（共通）` の直前に挿入する（フローの順序 explore → verify とルール節の順序を一致させる）:

```markdown
## 探索ルール（共通）

- 「どのファイルが関係するか」を絞り込む読み込みは、利用可能なら `Explore` サブエージェント（Agent tool で `subagent_type: Explore`）に委譲する。ファイル全文をメインコンテキストに載せずに当たりを付けるため。返ってきた候補が足りなければ観点を変えて再度起動する。`Explore` が使えない環境では自分で直接読む。
- **委譲する / しないの境界は「対象が特定済みか」で引く**。対象が未特定・3 ファイル以上に散る見込み・grep や find で当たりを付ける段階・設計書や外部リポジトリ／API 仕様の横断確認は委譲する。対象が 1〜2 ファイルに特定済み、読む量が事前に読める、編集直前の再 Read は委譲しない。verify 側の「出力量が事前に読めない場合は委譲する」と同じ基準。
- **確定した対象は main で Read する**。委譲で返るのは要約なので、実際に編集するファイルと、プランや報告に `file:line` として引用する箇所は必ずメインコンテキストで読み直して裏を取る。要約だけを根拠に編集やプランを書かない。
- 外部コマンド（`gh` / MCP / API spec ツール等）を叩く必要がある調査は `general-purpose` に委譲する。外部 API 仕様の一次情報確認は観点ごとに並列起動すると速い。
- 実装に入らない独立した原因調査は `investigate` スキル（scout → diver の 2 フェーズ）の領分。本スキルの explore にその手順を写さない。
- **委譲で解決しないものを委譲しない**。他エージェントの完了待ちに Agent tool を使わない。委譲プロンプトにサンドボックス無効化などの権限昇格指示を含めない（安全分類器にブロックされる）。
```

### `ai-agents/skills/dev-workflow/observations/amendments/2026-08-28_001_amendment.md`（新規）

`ai-agents/skills/skill-improve/template.md` のフォーマット（`## 分析サマリ` / `## 検出パターン` / `## 変更内容` / `## 前回 amendment の効果`）に従う。`version_before: 6` / `version_after: 7`。

- 分析サマリ: observations 69 件（2026-07-06 〜 2026-08-28）、failure 0 件。今回の起点は再発パターンではなく transcript のトークン実測である旨を明記する
- 検出パターン: 「書いてあるルールは 58% 言及 / 書いていないルールは 12% 言及」の非対称、委譲のやりすぎの害 3 件
- 期待される効果と次サイクルの評価軸: 後述「効果測定」の指標をそのまま置く
- 前回 amendment（v5 → v6）の効果: 本サイクルでは評価しない（v6 デプロイは 2026-08-24、observations が 4 件しか蓄積していない）。`references/verification-points.md` が読まれたかの評価は次サイクルへ持ち越す旨を記録する

### 変更しないもの

- **`ai-agents/agents.xml`**: dev-workflow を invoke させる入口だけを持ち、フェーズ内の手順は skill 側に置く分業になっている。explore 委譲はフェーズ内の手順なので skill に閉じる。ここに書くと 2 箇所メンテになり、`~/.claude/CLAUDE.md` として常時ロードされる分のトークンも増える
- **`AGENTS.md`**: リポジトリ構造・コマンド・規約を書く文書で、skill 内部の手順は範囲外
- **`ai-agents/skills/investigate/SKILL.md`**: dev-workflow 側から 1 行ポインタを張るだけで重複は解消する。`disable-model-invocation: true` の明示呼び出し専用 skill なので、dev-workflow から自動で流れ込むことはない
- **`ai-agents/agents/investigation-scout.md` / `investigation-diver.md`**: 使用実績 0 件だが、削除・統合の判断は本プランのスコープ外（Open questions に回す）
- **`.claude/rules/skill-authoring.md`**: 既に version bump と Observe ループを規定済み

## 効果測定

測定できない改善は評価できないので、行動指標（ルールが読まれたか）と成果指標（コンテキストが減ったか）を分ける。

### ベースライン（2026-08-28 実測、直近 30 日）

```sh
cd scripts/agent-stats
go run ./cmd/agent-stats --json --since 720h | jq '{
  sessions,
  agent_calls: ([.tools[] | select(.name == "Agent") | .count][0]),
  main_turns: .main.assistant_turns,
  sub_turns: .sub.assistant_turns,
  sub_turn_share: (.sub.assistant_turns / .assistant_turns * 1000 | floor) / 10,
  main_cr_per_turn: (.main.tokens.cache_read / .main.assistant_turns | floor),
  sub_cr_per_turn: (.sub.tokens.cache_read / .sub.assistant_turns | floor),
  main_read: .main.tool_counts.Read,
  sub_read: .sub.tool_counts.Read
}'
```

- `sessions` 128 / `agent_calls` 171 → **1.34 回/セッション**
- `sub_turn_share` **19.9%**
- `main_cr_per_turn` **153,325** / `sub_cr_per_turn` **52,152**（2.94 倍）
- `main_read` **1,649** / `sub_read` **1,220** → main 側 **57.5%**

委譲先の内訳は agent-stats が持たないので transcript を直接数える:

```sh
grep -roh '"subagent_type": *"[^"]*"' ~/.claude/projects/ | sed 's/.*: *//' | sort | uniq -c | sort -rn
```

- `Explore` 97 / `verify-runner` 25 / `Plan` 15 / `fork` 12 / `general-purpose` 8 / `investigation-scout` 0 / `investigation-diver` 0（全 167 件）

### 指標と目標

**一次指標（行動）** — ルールが読まれて行動が変わったかを直接示す。デプロイ後 2〜4 週で判定する。

1. `Agent` 呼び出し / セッション: 1.34 → **2.0 以上**
2. `subagent_type` の `Explore` 件数: 97 → 増加（新規セッション分で単調増加していること）
3. subagent ターン比: 19.9% → **25% 以上**
4. `Read` の main 比: 57.5% → **50% 未満**

**二次指標（成果）** — 一次指標が動いた場合にのみ意味を持つ。

1. `main_cr_per_turn`: 153,325 → **10% 程度の低下**（依頼元見積り −12.6% と整合するか）
2. compaction の `auto` trigger 件数（`.compactions`）: 減少方向

**三次指標（観測記録）** — 次サイクルの `/skill-improve dev-workflow` で数える。

1. explore を subagent に出したと記録された observation の比率: 現状 **8/69 = 11.6%** → verify-runner の 58% に近づくか
2. 「二重探索が発生しなかった」型の肯定的言及の件数

### 交絡と限界（測れないものを測れると言わない）

- `main_cr_per_turn` はセッション長・compaction 頻度・コンテキストウィンドウ設定に強く依存する。`docs/plan/2026-08-25_opus-5-bedrock-context-usage.md` の通り `ANTHROPIC_DEFAULT_OPUS_MODEL` の `[1m]` サフィックス有無で 200K / 1M が切り替わり、この値は大きく動く。**単独で因果を主張できない**ので二次指標に置く
- `--since 720h` は 30 日窓なので、デプロイ直後は変更前セッションが混ざる。`--since` をデプロイからの経過時間に合わせて狭め、同じ窓幅の前後比較を別途取る
- **「同じファイルの再読率」は現状の agent-stats では測れない**。`scripts/agent-stats/internal/parser/parser.go` の `fileEditTools` が `Edit` / `Write` / `MultiEdit` / `NotebookEdit` のみで、`Read` の `file_path` を集計していないため、`Top files` は編集回数であって読み込み回数ではない。依頼元が挙げた「論理セッション単位で 42.9% が同じファイルの再読」「`external-gateway.md` ×66、`dto.md` ×61」は agent-stats の出力ではなく別計測であり、本プランでは追試していない。フォローアップで agent-stats に scope 別の Read `file_path` 集計を足せば、explore 委譲の効果を最も直接的に測れるようになる
- `Explore` の tool_result サイズ（依頼元計測で平均 1,493 chars）も agent-stats は集計していない。委譲 1 回あたりの節約量を定量化するにはこれも必要

## Risks and mitigations

- **要約が薄くてプランの根拠が弱くなる**。`2026-08-14_002` で verify-runner が実際に起こした失敗と同型。→ 「確定した対象は main で Read する」「要約だけを根拠に編集やプランを書かない」を bullet として明記する。プランに書く `file:line` は main で確認したものに限る
- **往復が増えてターン数が増える**。委譲は起動と待機のオーバーヘッドを持つ。→ 「対象が 1〜2 ファイルに特定済みなら委譲しない」を発動条件に入れ、待機目的の Agent 起動（`2026-08-24_002`）を明示的に禁じる
- **委譲プロンプトが安全分類器にブロックされる**（`2026-08-07_002`）。→ 「権限昇格指示を委譲プロンプトに含めない」を明記
- **`Explore` は Claude Code 固有で Cursor / Codex / Copilot には無い**。SKILL.md は 4 CLI に配布される。→ verify-runner と同じ文型で「使えない環境では自分で直接読む」の縮退を書く
- **SKILL.md の分量増**。現在 10.0KB で、`hook-scaffold`（9.2KB）・`svleague-match-review`（11.3KB）に並ぶ規模。→ 追記を 6 bullet + 3 文に抑え、`references/` は増やさない
- **書いても行動が変わらない**。→ verify 側が 58% の言及率を得ている前例があり期待は持てるが、explore は Plan Mode というハーネス側レイヤーが絡むため同じ効きは保証されない。一次指標を行動指標に置いたのはこのリスクを検出するため。2〜4 週後に `Agent` 呼び出し数が動いていなければ、SKILL.md への記述では届かないと判断し、`agents.xml` への昇格か hook 化を検討する

## Validation

このタスク自体が「テストの無いタスク（ドキュメント）」なので、SKILL.md が規定する読み替えを自分に適用する。

1. **静的検証（変更ファイルにスコープ）** — リポジトリルートから `npx markdownlint-cli2 <変更した md>` と `npx prettier --check <変更した md>` を実行する。サブディレクトリへ `cd` すると markdownlint-cli2 が設定を見失い MD013 を誤検出する。`mise run lint:format` は `prettier --check .` と `markdownlint-cli2 "**/*.md"` を作業ツリー全体に対して回すため、無関係な未追跡ファイルに汚染される
2. **ベースライン差分** — 全体実行で残る指摘が HEAD 時点と同一であること（新規混入ゼロ）を示す
3. **frontmatter とリンクの整合** — `version: 7` になっていること、L47 / L55 から参照する「探索ルール（共通）」というセクション名が実在すること、既存の `references/verification-points.md` へのリンクを壊していないこと
4. **配布の確認** — `mise run skills-copy` 実行後に `grep -n "version:" ~/.claude/skills/dev-workflow/SKILL.md` が 7 を返し、`## 探索ルール（共通）` が配布先にも存在することを確認する。v5 の未デプロイ事故の再発防止として毎回行う
5. **ベースラインの再現** — 効果測定の 2 コマンドが実行でき、本プランに記載した数値を再現することを確認する（測定手段が壊れていないことの確認）
6. **実効性** — スキルは読まれて初めて効くため、次回 dev-workflow を起動した実タスクでの遵守が最終検証になる。即座には確認できないので **未検証として明示**する（v6 が定めた「どれも取れない場合は未検証であることを明示する」の実践）

## Open questions

- **`investigation-scout` / `investigation-diver` を残すか**。167 回の `Agent` 呼び出しと 114 件の observations を通して使用実績 0 件。dev-workflow から使わないと決めた今、`/investigate` 専用として残すのか、`Explore` に統合して定義を削るのかは別タスクの判断。`.claude/rules/agent-authoring.md` は「Scanner→Critic / Scout→Diver の 2 フェーズ分割を踏襲せよ」と書いているので、削除するならこのルールも見直しが必要
- **`Explore` に渡すプロンプトの型を用意するか**。`review` / `investigate` スキルは委譲プロンプトのテンプレートを持つが、explore の対象はタスクごとに大きく変わる。テンプレートを書くと空文化する恐れがあり、今回は書かない判断をした。2〜4 週後の観測で「委譲したが候補が的外れだった」型の記録が出たら再検討する
- **一次指標が動かなかった場合の昇格先**。`agents.xml`（常時ロード）に書くか、Plan Mode 開始時の hook にするか。前者はトークン常時課金、後者は Claude Code 固有になる
- **agent-stats の拡張スコープ**。scope 別の Read `file_path` 集計と `Agent` tool_result のサイズ集計を足せば explore 委譲の効果を直接測れる。`docs/plan/2026-08-22_agent-stats-observability-expansion.md` の続きとして別プランに切る
- **`~/.claude/skills/` に repo 管理外の skill が 6 件ある**（`hotfix-workflow` / `log-markdown-export` / `write-blog-entry` / `apply-review-feedback` / `review-fix` / `review-verify`）。`*-copy` は上書きのみで削除しないため蓄積する。本プランのスコープ外だが、これらが dev-workflow と重複する手順を持っていれば整合が必要
