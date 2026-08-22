# Plan: agent-stats の観測範囲拡張と Bash 偏重の是正

## Background

`scripts/agent-stats/` を全 transcript（234 セッション / 7,772 turns / output 7.76M tokens）に対して実行し、生 transcript との突き合わせ調査を行った。結論は 2 つある。

第一に、PDE 側に明確な非効率がある。最大のものは「専用ツールで済む操作を Bash で代替している」こと。Bash 5,145 回に対し Grep 27 回 / Glob 6 回。Bash コマンド文字列に含まれる数は `grep` 1,530 / `head -` 994 / `cat` 844 / `tail -` 725 / `find` 525 / `sed` 283。さらに先頭 `cd` が 1,008 件あり、これは harness が「permission prompt を誘発する」と明示的に警告しているパターンである。実際に権限拒否 31 件、`guard-dangerous` hook ブロック 11 件、非 0 exit 約 234 件が発生している。

第二に、その非効率を agent-stats では追跡できない。transcript には未使用の信号が大量にあり、改善を打っても効果を定点観測できない。

よって「計測を先に直し、そのうえで PDE を直す」順で進める。計測のない改善は効果検証ができず、このツールを作った目的（ローカル観測）に反する。

意図する成果は、`agent-stats` 単体で main と subagent の切り分け・実作業時間・エラー率・Bash 内訳・プロジェクト別が読め、次の改善サイクルの before/after が取れる状態にすること。

## Current structure

```text
scripts/agent-stats/
  cmd/agent-stats/main.go     # フラグ解釈 → 走査 → 集計 → 出力
  internal/parser/            # JSONL → Session
  internal/report/            # 集計 + table / json 整形
```

`parser.applyLine` は `type=="assistant"` の行のみを処理し、`message.usage` を `message.id` 単位で dedupe して積む。`report.Summarize` はセッション列を畳み込み、`Models` / `Tools` / `Files` / `Skills` の順位表を作る。mise task（`agent-stats:build|test|lint|clean`）と `.github/workflows/ci_agent_stats.yml` は既に整備済み。

### 観測盲点（すべて transcript に存在するのに未使用）

| 信号                                                 | 実測値                                                                  | 現状                                                                                                              |
| ---------------------------------------------------- | ----------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `isSidechain`                                        | subagent turn 1,298 / 全 7,777（16.7%）、output 212K                    | main と合算されており分離不可                                                                                     |
| `system/turn_duration.durationMs`                    | 504 turn 分、実作業 165.3h（平均 19.7 分/turn）                         | 未使用。代わりに first-to-last span（1,278h、idle 込み）を出し、README で自ら「実作業時間ではない」と注記している |
| `system/compact_boundary.compactMetadata`            | 36 回（manual 22 / auto 14）、平均 preTokens 180〜217K、累計 drop 16.3M | 未使用。コンテキスト圧が見えない                                                                                  |
| `system/stop_hook_summary`                           | 517 件、`hookErrors` 計 42                                              | 未使用。検証 hook の成否が見えない                                                                                |
| `type:"user"` の `tool_result.is_error`              | 非 0 exit 約 234、権限拒否 31、hook ブロック 11                         | 未使用（parser は assistant 行しか見ない）。エラー率が出ない                                                      |
| `message.model` 単位のトークン按分                   | sonnet-5 5.02M / opus-5 1.67M / opus-4.8 1.05M output                   | `Models` はセッション数の数え上げのみ。opus はセッション数 27% だがトークンでは 35%                               |
| Bash コマンドの内訳                                  | 上記のとおり                                                            | 最大ツールが 5,145 という単一の不透明バケツ                                                                       |
| `ai-title.aiTitle` / `agent-name.agentName` / `slug` | 2,212 / 560 件                                                          | セッションは UUID ファイル名表示で判読不能                                                                        |
| プロジェクト別                                       | cch 99 / llm-gw-terraform 60 / shirousagi-be 24 …                       | `--project` フィルタはあるが内訳集計はない                                                                        |

### 小さいが実在する不具合

- `Session.Model` が last-write-wins。2 モデル混在セッション 15 件のうち 4 件で支配的モデルと不一致。
- `<synthetic>` が 8 件、実モデルと同列で `Models` に混入している（usage は 0 なのでトークンには影響しない）。
- `cmd/agent-stats/main.go:84` の WalkDir コールバック冒頭 `return err` により、ディレクトリ読み取りエラー 1 件で走査全体が中断する。ファイル単位では skip して続行する丁寧な設計（同 94 行）と非対称。

### 調査で否定した仮説

resume / compact による transcript 間の二重カウントを疑ったが、複数ファイルに現れる `message.id` は 7,778 件中 1 件のみだった。現行のファイル単位 dedupe で十分であり、ここには手を入れない。

## Design policy

- **parser 隔離を守る。** transcript スキーマを知るのは `internal/parser` だけ。新しい行種（`system` / `user` / `ai-title` / `agent-name`）の解釈もここに閉じ、レポート層へ波及させない。寛容パースの方針（未知フィールド無視・壊れ行 skip）は維持する。
- **過大申告しない。** `turn_duration` は新しめの CLI しか書かないため、カバーした turn 数を必ず併記する。Bash の「専用ツールで代替可能」判定は、先頭コマンドが該当する場合のみ数える（`cmd | grep` のような正当なパイプラインを誤検出しない）。
- **単価をコードに焼き込まない。** コスト概算は有用だが単価は腐る。かつこの環境は `CLAUDE_CODE_USE_BEDROCK=1` で Bedrock 課金であり API 価格と異なる。`--prices <file>` を渡したときだけ算出する任意機能に留め、単価表はリポジトリにコミットしない。
- **PDE 側の是正は非破壊から始める。** Bash 偏重には、まず `agents.xml` のソフトな誘導と計測指標を入れる。PreToolUse hook による強制は正当なパイプラインまで阻害しうるため、効果を測ってから次サイクルで判断する。

## Implementation steps

### 1. 計測の正確性（parser / report）

`Session` のツール・トークン・turn 集計を、出自ごとに分けた構造体へ切り出す。

```go
// Scope is the per-origin (main loop vs subagent) aggregate.
type Scope struct {
    Tokens         Tokens            `json:"tokens"`
    TokensByModel  map[string]Tokens `json:"tokens_by_model"`
    AssistantTurns int               `json:"assistant_turns"`
    ToolCounts     map[string]int    `json:"tool_counts"`
    FileCounts     map[string]int    `json:"file_counts"`
    SkillCounts    map[string]int    `json:"skill_counts"`
    BashCounts     map[string]int    `json:"bash_counts"`
}
```

`Session` は既存の `File` / `Cwd` / `GitBranch` / `Start` / `End` を据え置き、`Main Scope` と `Sub Scope` を持つ。`applyLine` は `isSidechain` で振り分け先を決める（欠落行は `false` 扱い。実測で 4 行のみ）。dedupe 用 `seenMessageIDs` は Session 単位のまま維持する。

`Session.Model`（last-write-wins）は廃止し `Scope.TokensByModel` に按分する。`<` で始まる model 値は実モデルでないためモデル集計から除外し、件数だけ注記する。`report.Summary.Models []Count`（セッション数）は `ModelTokens`（turn 数 + output / cache トークン）へ差し替える。これが「opus のセッション数は少ないがトークン消費は大きい」を可視化する本体。

`type=="system" && subtype=="turn_duration"` の `durationMs` を合算して `Session.ActiveDuration` とし、カバー turn 数と併せてレポートに 2 行で出す。

```text
Active time:     165h18m (sum of turn_duration over 504 recorded turns)
Session span:    1278h11m (first-to-last timestamp, includes idle in resumed sessions)
```

### 2. 非効率の可視化

Bash の `tool_use` から `input.command` を取り、**1 行目の先頭トークン**を正規化キーにして `Scope.BashCounts` に積む。複数行 heredoc で `EOF` や `###` を拾う誤集計を避けるため先頭行のみを見る（調査時に実際に踏んだ罠）。`cd` / `env VAR=…` のような前置は読み飛ばして実コマンドまで進める。

併せて「専用ツールで代替可能」判定を別カウンタで持つ。

```go
// bashRedundant maps a shell command to the dedicated tool that should replace it.
var bashRedundant = map[string]string{
    "cat": "Read", "head": "Read", "tail": "Read",
    "grep": "Grep", "rg": "Grep", "find": "Glob", "ls": "Glob",
}
```

`Bash breakdown` セクションと `Redundant Bash: N (Read: n, Grep: n, Glob: n)` の 1 行サマリを出す。これがステップ 5 の before / after 指標になる。

parser に `type=="user"` の `tool_result` 処理を追加し `is_error==true` を数える。内訳は content 先頭のパターンで `permission`（`The user doesn't want to` で始まる）/ `hook`（`PreToolUse:` / `PostToolUse:` を含む）/ `failure`（それ以外）に 3 分類する。文字列マッチはヒューリスティックなのでその旨を `parser.go` にコメントし、分類不能は `failure` に落とす。

`subtype=="compact_boundary"` から `trigger` / `preTokens` / `cumulativeDroppedTokens` を採り `Compactions` セクションを出す。auto compaction は「一度に抱え込みすぎている」直接証拠なので、後の改善対象を選ぶ材料になる。

### 3. 読みやすさと運用

- `ai-title.aiTitle` → `agent-name.agentName` → `slug` → ファイル名（UUID）の順にフォールバックして `Session.Title` を埋める。
- transcript の親ディレクトリ名または `Cwd` から `Session.Project` を導出し `By project` セクション（セッション数 / turn / output tokens）を追加する。既存の `--project` フィルタはそのまま残す。
- `Top sessions`（output tokens 降順、Title 付き）を table に追加する。現状 `List` は JSON にしか出ておらず、table では「どのセッションが重かったか」が分からない。
- `Summary.List` が常に全セッション分入るため `--json` が巨大になる。`--json` は集計のみ、`--json --detail` で `List` を含める、に分ける。
- `main.go:84` の `return err` を、ディレクトリなら stderr 警告 + `fs.SkipDir`、ファイルなら skip して継続に変更し、ファイル単位の丁寧さと対称にする。

### 4. テスト

`internal/parser/parser_test.go` の既存パターン（インライン JSONL 文字列を `ParseReader` に流す）を踏襲して追加する。

- `TestParseReaderSplitsSidechain` — main / sub 両方の行を含む入力で正しく振り分かれる
- `TestParseReaderTokensByModel` — 同一セッション 2 モデルで按分され `<synthetic>` が除外される
- `TestParseReaderActiveDuration` — `turn_duration` の合算とカバー turn 数
- `TestParseReaderToolErrors` — `is_error` の 3 分類
- `TestParseReaderBashBreakdown` — heredoc を含む複数行コマンドで先頭行のみを見ること、`cd` 前置の読み飛ばし

`internal/report/report_test.go` の `TestSummarize` / `TestRenderTable` を新スキーマに追随させる。

### 5. PDE 側の是正

`ai-agents/agents.xml` に `<tool_selection>` セクションを追加する。`~/.claude/CLAUDE.md` は `mise run claude-link` でこのファイルの symlink になっているため、ここを直せば全プロジェクトに効く。要点は次の 4 点。

- ファイル内容を見る目的の `cat` / `head` / `tail` は `Read` を使う
- コード検索の `grep` / `rg` は `Grep`、ファイル名探索の `find` / `ls` は `Glob` を使う
- `cd` を前置せず絶対パスを渡す（permission prompt を誘発する）
- Bash は実行が必要な処理（git / gh / mise / go / terraform / テスト実行）に限る

harness 自身も同趣旨の指示を持つが実測で守られていないため、プロジェクト横断の常設ルールとして重ねる。

`AGENTS.md` にも実態との乖離がある（`AGENTS.md:32` の Go apps 一覧に `agent-stats` / `scaffold` が漏れている、`AGENTS.md:10-19` の Project Structure に `scripts/agent-stats/` の行がない、`AGENTS.md:68,74` が空になった `~/.claude/rules/` を参照したまま）。ただし AGENTS.md の更新は専用の `ai-agents/skills/update-agents-md/` 経由で別途対応する方針のため、本計画では `AGENTS.md` を変更しない。

### 6. 今回やらないこと（計測してから決める）

- **Bash 誘導の hook 強制。** ステップ 5 のソフトな誘導とステップ 2 の `Redundant Bash` 指標で効果を見る。不十分なら次サイクルで PreToolUse hook を検討する。
- **`dev-workflow` skill の発火率。** `agents.xml` は非自明な実装で必須と書いているが実測は 234 セッション中 34 回。ただし「非自明な実装だったセッション」の分母が現状のデータでは取れない。既存の `skill-observe` / `skill-improve` を回して分母付きで評価する。
- **auto compaction 14 回への対処。** ステップ 2 でプロジェクト別・セッション別に出してから原因（大きいファイルの全読み、subagent 未活用など）を特定する。
- **コスト概算。** ステップ 1〜3 完了後の任意項目とする。

## File changes

| パス                                                 | 変更内容                                                                                                                                                                                     |
| ---------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `scripts/agent-stats/internal/parser/parser.go`      | `Scope` 導入、`isSidechain` 振り分け、モデル別按分、`system` / `user` 行の解釈、Bash 内訳、`Title` / `Project` 導出                                                                          |
| `scripts/agent-stats/internal/parser/parser_test.go` | 上記のテスト追加                                                                                                                                                                             |
| `scripts/agent-stats/internal/report/report.go`      | `Summary` スキーマ更新、`ModelTokens` / `Bash breakdown` / `Tool results` / `Compactions` / `By project` / `Top sessions` セクション、Active time と Session span の 2 行化、`--detail` 対応 |
| `scripts/agent-stats/internal/report/report_test.go` | 新スキーマ追随                                                                                                                                                                               |
| `scripts/agent-stats/cmd/agent-stats/main.go`        | `--detail`（および任意で `--prices`）追加、WalkDir のエラー扱い修正                                                                                                                          |
| `scripts/agent-stats/README.md`                      | Duration の注記を実作業時間の説明に差し替え、新セクションを追記                                                                                                                              |
| `ai-agents/agents.xml`                               | `<tool_selection>` 追加                                                                                                                                                                      |

コミット単位は、ステップ 1 + 4（スキーマ変更のため parser / report のテストが同時に動くので分割しない）→ ステップ 2 + 4 → ステップ 3 + README → ステップ 5 の 4 本。

## Risks and mitigations

- **`Session` スキーマ変更が `report` を巻き込む。** ステップ 1 で parser と report のテストを同時に通し、中間状態をコミットしない。
- **`--json` の出力形状が変わる。** 現状この JSON を消費する下流は存在しない（`grep -rn "agent-stats" mise.toml .github/workflows .claude/hooks` の結果はビルド / テスト / lint task と CI のみ）。破壊的変更を許容し、README に新形状を記載する。
- **`turn_duration` の欠測を「実作業時間が短い」と誤読する。** 出力に必ずカバー turn 数（504 / 7,772）を併記する。
- **エラー分類の文字列マッチが CLI 更新で壊れる。** 分類不能は `failure` に落として総数は必ず正しく保ち、分類内訳のみ劣化させる。
- **`agents.xml` の指針追加が正当な Bash 利用を萎縮させる。** hook で強制せず指針に留める。`Bash breakdown` で git / gh / mise / go の件数が不自然に落ちていないか次サイクルで確認する。

## Validation

各ステップ完了時に以下を通す。

```sh
mise run agent-stats:test     # go test ./...
mise run agent-stats:lint     # golangci-lint + goimports
```

実データでの動作確認（read-only なので安全）。

```sh
cd scripts/agent-stats
go run ./cmd/agent-stats                      # 全件
go run ./cmd/agent-stats --since 168h         # 直近 7 日
go run ./cmd/agent-stats --project pf-customer-context-hub
go run ./cmd/agent-stats --json | jq '.tokens, .model_tokens'
```

回帰の期待値として、今回の調査で jq により独立に算出した実測値を使う。実装はこれに一致すべき。

| 項目                             | 期待値                                                     |
| -------------------------------- | ---------------------------------------------------------- |
| main turns / sub turns           | 6,479 / 1,298                                              |
| main output / sub output         | 7,551,131 / 212,287                                        |
| output by model                  | sonnet-5 5,023,744 / opus-5 1,674,866 / opus-4.8 1,053,400 |
| turn_duration                    | 504 turn, 165.3h                                           |
| compactions                      | manual 22 / auto 14                                        |
| Bash 総数 / 先頭 grep / 先頭 cat | 5,145 / 755 / 241                                          |

ステップ 5 のデプロイ確認。

```sh
mise run claude-link
readlink ~/.claude/CLAUDE.md   # ai-agents/agents.xml を指していること
```

ステップ 5 の効果は次回以降のセッションで `Redundant Bash` と `Grep` / `Glob` の比率が改善しているかで判定する。

## Open questions

着手前に確認した 2 点はいずれも確定済み。残る未解決の問いはない。

- **`AGENTS.md` の追随は本計画では行わない。** `ai-agents/settings/claude/rules/` の 5 ファイル削除により `~/.claude/rules/` が空になった一方 `AGENTS.md:68,74` は参照を残しているが、AGENTS.md の更新は専用の `ai-agents/skills/update-agents-md/` 経由で別途対応する。Go apps 一覧の漏れも同様に扱う。
- **Bash 偏重への対処は指針 + 計測に留める。** PreToolUse hook による強制は本サイクルでは行わず、ステップ 5 の指針とステップ 2 の `Redundant Bash` 指標で効果を測ってから次サイクルで判断する。
