# Plan: agent-stats の計測欠陥修正

## Background

`scripts/agent-stats` の集計をもとに運用改善施策を 4 つ立てたが、4 施策すべてがレビューで却下された。却下の主因は施策の中身ではなく、**agent-stats の計測が壊れており、誤った数字に基づいて診断していたこと**である。4 人のレビュアーが独立に別々の欠陥を指摘した。

本計画に着手する前に、指摘された欠陥をすべて現行 transcript（2026-08-28 時点、297 ファイル / assistant turn 9,341 / tool_result 11,719 / `is_error` 405）に対して独立に再計算し、裏付けを取った。以下の数値はすべてその実測値であり、レビュー時点のスナップショットとは母数が異なる（transcript は使うほど増える）。

誤診の中核は 3 つある。

第一に、**エラー分類が権限拒否を取りこぼしている。** `classifyToolError` は `Permission to use Bash with command ... has been denied` と `Permission for this action was denied by the Claude Code auto mode classifier` のどちらも認識せず、`failure` に混ぜている。実測では真の拒否 44 件（Bash 32 / Read 10 / Agent 2）が `failure` 側に埋まっていた。逆に現在 `permission` として報告している 42 件の内訳は ExitPlanMode 31 / AskUserQuestion 8 / Bash 3 で、大半は「摩擦」ではなく人間の意思表示である。両者を同じバケツに入れている限り、どちらの数字も打ち手に結びつかない。

第二に、**tool_result のサイズを一切記録していない。** `Scope` は回数とトークンしか持たないため「どのツールが何トークン context に流し込んだか」が原理的に出せない。この欠落のせいで「Bash が全ツール呼び出しの 51.4%」という**回数シェア**をコンテキスト圧の指標として誤読した。実測では文字数シェアは Read 52.1% / Bash 35.3% で逆転する（1 回あたり Read 5,065 chars vs Bash 1,684 chars）。回数とサイズは別の指標であり、後者が無いままでは cache_read の議論ができない。

第三に、**集計単位が context ではない。** 現在の main / sub 集約では「subagent の tool_result は親の cache_read を増やさない」という事実が消える。実測で main は 152,327 tokens/turn、subagent は 51,027 tokens/turn（2.99 倍差）であり、これが施策判断の決め手になる数字だった。同じ理由で、各 context の 1 turn 目に載る固定 overhead（system prompt + ツールスキーマ + CLAUDE.md + skill 一覧）が完全に不可視のままになっている。実測中央値は main context で 49,674 tokens ある。

意図する成果は、`agent-stats` 単体で「context を膨らませている実体」を tool 別・skill 別・固定 overhead 別に切り分けて読めるようにし、次の施策を回数シェアではなくトークン負荷に基づいて立てられる状態にすること。施策そのものは本計画の対象外とする。

## Current structure

```text
scripts/agent-stats/
  cmd/agent-stats/main.go     # フラグ解釈 → 走査 → 集計 → 出力
  internal/parser/            # JSONL → Session
  internal/report/            # 集計 + table / json 整形
```

`parser.Session` は `Main Scope` / `Sub Scope` の 2 スコープを持ち、`isSidechain` で振り分ける。`Scope` は `Tokens` / `AssistantTurns` / `ByModel` / `ToolCounts` / `FileCounts` / `SkillCounts` / `BashCounts` / `BashWithCd` / `ToolResults` / `ToolErrors` を持つ。`applyAssistant`（`parser.go:436`）が `tool_use` を、`applyUser`（`parser.go:510`）が `tool_result` を処理し、`applySystem`（`parser.go:528`）が `turn_duration` と `compact_boundary` を拾う。`report.Summarize`（`report.go:117`）が全セッションを畳み込み、`RenderTable` / `RenderJSON` が出力する。

### transcript 側で確認した構造（実測）

| 事実                                              | 実測                                                | 意味                                             |
| ------------------------------------------------- | --------------------------------------------------- | ------------------------------------------------ |
| `tool_use` は必ず `id` を持つ                     | サンプル 1,311/1,311                                | join のキーが確実に存在する                      |
| `tool_result` は必ず `tool_use_id` を持つ         | サンプル 1,310/1,310                                | 同上                                             |
| 1 つの user 行に `tool_result` は 1 個だけ        | 11,695 turn すべて 1 個                             | 按分不要。turn 単位の帰属が一意に決まる          |
| `tool_result.content` は文字列か blob 配列        | str 11,216 / list 433                               | list の中身は text 307 / tool_reference 236      |
| 1 ファイルに main と sidechain が混在しない       | mixed 0 / main のみ 125 / sidechain のみ 165 / 空 7 | context == 1 transcript ファイルが厳密に成立する |
| `attributionSkill` は assistant 行の最上位 string | 6,575 行、type は全て assistant                     | skill 実行中ターンを完全に特定できる             |
| `tool_use.caller` は `direct` のみ                | 445 件すべて                                        | skill 判別には使えない                           |

### 欠陥ごとの現状と差分

| ID  | 欠陥                     | 現状のコード                                              | 実測での影響                                                                          |
| --- | ------------------------ | --------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| A   | 権限拒否の取りこぼし     | `classifyToolError`（`parser.go:561`）                    | 真の拒否 44 件が `failure` に混入。`permission` 42 件は実は人間の意思表示             |
| B   | tool_result サイズ未記録 | `applyUser` は `ToolResults++` と `IsError` のみ          | 回数シェア（Bash 51.4%）とサイズシェア（Read 52.1%）が逆転しているのに検証不能        |
| C   | cache_read の帰属不明    | 該当コードなし                                            | 「どのツールが cache_read を膨らませているか」を検証も反証もできない                  |
| D   | 固定 overhead 不可視     | 該当コードなし                                            | main context の中央値 49,674 tokens、cache_read の最大項が見えない                    |
| E   | context 単位の集計なし   | `Scope` は main / sub の 2 つだけ                         | main 152,327 vs sub 51,027 tokens/turn の 2.99 倍差が消える                           |
| F   | サブコマンド分解不足     | `LeadingCommand`（`parser.go:612`）は先頭 1 トークンのみ  | `git add && git commit` の commit を落とす。commit 100→139、push 118→123 で順位が逆転 |
| G   | `attributionSkill` 未読  | `rawLine`（`parser.go:270`）に無い                        | Skill tool_use 80 に対し実際の skill 起動は 141。skill 起動数を大幅に過小計測         |
| H   | Active time の誤読誘発   | `turnStats`（`report.go:181`）/ 見出し（`report.go:338`） | カバレッジ 5.5%（512/9,341）で合計 168h23m を提示。上位 5 ターンで合計の 74.6%        |
| I   | Read の作法未計測        | Read は `fileEditTools` に含まれず素通り                  | offset/limit なしの全文 Read が 2,356/2,938 calls（80.2%）                            |

### F の実測比較

| subcmd | 現行（先頭方式） | 全セグメント方式 |
| ------ | ---------------- | ---------------- |
| status | 190              | 422              |
| log    | 108              | 338              |
| diff   | 113              | 277              |
| commit | 100              | 139              |
| push   | 118              | 123              |
| add    | 106              | 120              |

先頭方式では push 118 > commit 100 だが、全セグメント方式では commit 139 > push 123 になる。「やり直し push が多い」という誤診はこの artifact に由来する。

## Design policy

- **parser 隔離を守る。** transcript スキーマを知るのは `internal/parser` だけ。新しいフィールド（`id` / `tool_use_id` / `attributionSkill`）の解釈もここに閉じる。寛容パース（未知フィールド無視・壊れ行 skip・欠落フィールドは静かに degrade）は維持する。
- **測定とモデルを混ぜない。** tool_result の**文字数**は測定値なので生のまま出す。cache_read の帰属は差分法による**モデル**なので、モデルであることを名前と README で明示し、恒等式との差（over-attribution）を必ず併記する。トークン数を文字数から係数で推定することはしない（assistant 出力で校正した chars/token は 0.99 で、英語のファイル内容には転用できない）。
- **既存指標の意味を変えない。** `BashCounts`（先頭方式）は残し、サブコマンド分解は別フィールドで足す。`median_turn_ns` / `longest_turn_ns` も残して p90 / p95 を足す。名前が嘘になる箇所だけを直す（`tool_errors` のバケツ名）。
- **外れ値を除外しない。** 88h35m のターンは実在の事象であり、除外すると別の嘘になる。p95 と max を並記し、カバレッジ率を明示して読者に判断させる。
- **未文書フィールドに依存しても壊れない。** `attributionSkill` は CLI 2.1.218〜2.1.248 で確認しただけの未文書フィールド。無ければ空のまま出力側で「記録なし」と表示する。
- **パスは出すが内容は出さない。** Read の重複計測でファイルパスを出すのは、既存の `FileCounts` が編集ファイルのパスを出しているのと同じ水準に留める。プロンプト本文や tool_result 本文は出さない。
- **フェーズを分ける。** A は数行で即効、C と E は設計変更を伴う。1 コミットに詰めず、依存の順に積む。

## Implementation steps

### フェーズ構成と依存

| Phase | 内容                                          | 規模の目安               | 依存 | 壊すリスク                          |
| ----- | --------------------------------------------- | ------------------------ | ---- | ----------------------------------- |
| 1     | A 分類是正 + H Active time 表示               | parser +40 / report +70  | なし | 低（局所）                          |
| 2     | B tool_use join + 出力量 + F サブコマンド分解 | parser +140 / report +90 | なし | 低（純加算）                        |
| 3     | E context 単位 + D baseline                   | parser +120 / report +80 | 2    | 中（`AppendReader` シグネチャ変更） |
| 4     | C 位置重み付き cache_read 帰属                | parser +130 / report +90 | 2, 3 | 中（モデル化と恒等式検証）          |
| 5     | G attributionSkill + I Read 作法              | parser +100 / report +80 | 2, 3 | 低                                  |

B が C の前提であるのは、帰属先のツール名が tool_use id の join でしか決まらないため。E が C の前提であるのは、位置重み（残存ターン数）が context 単位の turn 列と compaction 境界の位置に依存するため。F が G の前提であるのは、skill 別の git サブコマンド内訳がサブコマンド分解の上に載るため。

### Phase 1: 誤分類の停止と Active time の表示是正

`internal/parser/parser.go` のエラー種別定数を入れ替える。`ErrPermission` は削除し、人間の意思表示と権限システムの拒否を分ける。

```go
const (
    ErrHook             = "hook"
    ErrUserReject       = "user_reject"
    ErrPermissionDenied = "permission_denied"
    ErrInvalidInput     = "invalid_input"
    ErrFailure          = "failure"
)
```

`classifyToolError` は判定順を固定する。hook を最優先にするのは、hook のメッセージ本文が拒否文言を引用しうるため。

```go
func classifyToolError(text string) string {
    switch {
    case strings.Contains(text, "PreToolUse:"), strings.Contains(text, "PostToolUse:"):
        return ErrHook
    case strings.HasPrefix(text, "The user doesn't want to"),
        strings.Contains(text, "tool use was rejected"):
        return ErrUserReject
    case strings.HasPrefix(text, "Permission to use"),
        strings.Contains(text, "denied by the Claude Code auto mode classifier"),
        strings.Contains(text, "denied by your permission settings"):
        return ErrPermissionDenied
    case strings.Contains(text, "InputValidationError"):
        return ErrInvalidInput
    default:
        return ErrFailure
    }
}
```

`Permission to use` は実測 31/31 が tool_result の先頭に出るため `HasPrefix` で足りる（本文中にのみ現れる例は 0 件）。この分類での実測内訳は hook 8 / user_reject 42 / permission_denied 44 / invalid_input 8 / failure 303（計 405）。現行は permission 42 / hook 8 / failure 355。

`internal/report/report.go` の `turnStats` は戻り値を struct にする。3 値の位置戻しに p90 / p95 を足すと呼び出し側が読めなくなる。

```go
// TurnStats describes the pooled turn timings together with the spread that
// makes the total meaningless on its own.
type TurnStats struct {
    Count   int           `json:"count"`
    Total   time.Duration `json:"total_ns"`
    Median  time.Duration `json:"median_ns"`
    P90     time.Duration `json:"p90_ns"`
    P95     time.Duration `json:"p95_ns"`
    Longest time.Duration `json:"longest_ns"`
}

func turnStats(turns []time.Duration) TurnStats
```

`Summary` には既存の `ActiveDuration` / `ActiveTurns` / `MedianTurn` / `LongestTurn` を残したまま `P90Turn` / `P95Turn` / `ActiveCoverage float64` を足す。パーセンタイルは `sorted[min(len-1, int(float64(len-1)*p))]` の nearest-rank で固定し、定義を godoc に書く。

`RenderTable` の見出しは 2 行に分け、カバレッジが `minActiveCoverage`（0.5）未満のときだけ警告を出す。実測値では次のようになる。

```text
Active time:     168h23m over 512 of 9341 turns (5.5% coverage)
                 median 1m33s  p90 14m2s  p95 23m11s  longest 88h35m15s
                 -> coverage is under 50%: the total is not working time
```

`writeTopSessions`（`report.go:471`）の `active` 列は同じ歪みを持つ（104 ターンで 89h4m のセッションが実在する）。列は消さず、表示を median に差し替えてカバーターン数を添える。JSON の `active_duration_ns`（合計）はそのまま残し、`active_median_ns` と `active_turns` を足す。合計は headline に既にあり、セッション行で読みたいのは典型値だという判断。

```text
  output 412331     turns 214    active med 45s (104 turns)  the heavy one
```

### Phase 2: tool_result の帰属基盤とサブコマンド分解

`rawContent` に join 用の 2 フィールドを足す。

```go
type rawContent struct {
    Type      string          `json:"type"`
    ID        string          `json:"id"`
    ToolUseID string          `json:"tool_use_id"`
    Name      string          `json:"name"`
    Input     json.RawMessage `json:"input"`
    IsError   bool            `json:"is_error"`
    Content   json.RawMessage `json:"content"`
}
```

`Session` に非公開の対応表を持たせ、`applyAssistant` で登録して `applyUser` で引く。`seenMessageIDs` と同様に `NewSession` と `AppendReader` で初期化する。

```go
type toolCall struct {
    Tool    string
    Command string
}
```

`Session.pendingCalls map[string]toolCall` に `id → {ツール名, LeadingCommand キー}` を入れる。分割ターン（同一 `message.id`）でも `tool_use` が現れる行は 1 行だけなので、dedupe の有無に関わらず登録して問題ない。

`Scope` には 3 つのフィールドを足す。回数・バイト数・エラー数を並列の 3 マップにすると片方だけ更新される事故が起きるため、値を struct にする。

```go
// ResultVolume is how much one tool fed back into the context.
type ResultVolume struct {
    Results int `json:"results"`
    Bytes   int `json:"bytes"`
    Errors  int `json:"errors"`
}
```

```go
ToolResultVolume map[string]ResultVolume    `json:"tool_result_volume"`
BashResultVolume map[string]ResultVolume    `json:"bash_result_volume"`
ToolErrorsByTool map[string]map[string]int  `json:"tool_errors_by_tool"`
```

`BashResultVolume` は `LeadingCommand` のキーで引く。`ToolErrorsByTool` は種別 → ツール名の 2 段。`ensure()` に 3 つの nil 初期化を足し、`Merge()` に `mergeVolumes(dst, src map[string]ResultVolume)` と `mergeNested(dst, src map[string]map[string]int)` を足す。`mergeNested` は Phase 2 の `ToolErrorsByTool`、F の `SubcommandCounts`、Phase 5 の `SubcommandBySkill` の 3 箇所で使う。

サイズは新しい `toolResultSize(raw json.RawMessage) int` で測る。既存の `toolResultText` は分類用に「最初の text ブロック」を返す実装なので流用できない。`toolResultSize` は文字列ならその長さ、配列なら全 text ブロックの長さの合計を返す（`tool_reference` ブロックは text を持たないので 0）。`tool_use_id` が引けない場合は `(unknown)` に寄せ、`ToolResults` の総数は従来どおり正確に保つ。

F のサブコマンド分解は `LeadingCommand` を壊さずに足す。まず既存の前置スキップ部分を切り出す。

```go
func commandName(segment string) (name string, withCd bool)
func CommandSegments(cmd string) []string
func Subcommand(segment string) (name, sub string)
```

`LeadingCommand` は `commandName` を使う形に書き換え、外部から見た挙動を変えない（既存の `TestLeadingCommand` が無変更で通ることが回帰ガードになる）。`CommandSegments` は heredoc を落としてから `&& || ; |` と改行で分割する。heredoc の除去は最初の `<<` でコマンド文字列を切る方式にする。実測でこれで足りる。

`Subcommand` は誤検出とカーディナリティ爆発を避けるため、複数動詞を持つコマンドの allowlist を通す。`cat foo.txt` の `foo.txt` をサブコマンドとして記録すると、出力にファイルパスが際限なく並ぶ。

```go
var subcommandHosts = map[string]bool{
    "git": true, "gh": true, "mise": true, "go": true, "docker": true,
    "kubectl": true, "npm": true, "terraform": true, "cargo": true,
    "gcloud": true, "aws": true, "helm": true, "brew": true, "jj": true,
}
```

サブコマンドは host 名の後ろ 3 トークンまでを走査し、`^[a-z][a-z0-9-]*$` に一致する最初のトークンを採る。`git -C /x status` は `/x` が正規表現に落ちるので `status` を拾える。`git -C x status`（相対パス）は誤って `x` を拾うが実測での出現はなく、README の限界として書く。

引用符内の `|` でセグメントが割れる問題（`git commit -m "fix: a|b"`）は、セグメントに host 名が含まれない限り何も記録しないという性質で無害化される。テストで固定する。

```go
SubcommandCounts map[string]map[string]int `json:"subcommand_counts"`
```

`report.Summary` には出力量とサブコマンドのセクションを足す。ネストしたマップの整形は 1 つのヘルパに寄せる。

```go
// Breakdown is a named group of counts with its own total, for the nested
// maps a flat ranking cannot represent.
type Breakdown struct {
    Name  string  `json:"name"`
    Total int     `json:"total"`
    Items []Count `json:"items"`
}

func rankedNested(m map[string]map[string]int, limit int) []Breakdown
```

table は回数とサイズを並べ、逆転が一目で分かる形にする。

```text
Tool output volume (28547116 chars over 11719 results)
  Read     14876882 (52.1%)   2940 results   avg 5060
  Bash     10066995 (35.3%)   6029 results   avg 1670
  -> calls and volume rank differently; Bash is 51.4% of results but 35.3% of volume
```

### Phase 3: context 単位の集計と baseline

context == 1 transcript ファイルが厳密に成立する（実測で混在 0 件）ため、`AppendReader` の呼び出し 1 回を 1 context として扱う。context 名が必要なのでシグネチャを変える。

```go
func AppendReader(s *Session, name string, r io.Reader)
```

呼び出し側は `AppendFile`（`filepath.Base(path)` を渡す）と `ParseReader`（受け取った name を渡す）とテストのみ。

```go
// ContextStats is one transcript file: the unit a context window spans.
type ContextStats struct {
    File           string `json:"file"`
    IsSidechain    bool   `json:"is_sidechain"`
    Turns          int    `json:"turns"`
    Tokens         Tokens `json:"tokens"`
    BaselineTokens int    `json:"baseline_tokens"`
    Compactions    int    `json:"compactions"`
    ToolResults    int    `json:"tool_results"`
    ResultBytes    int    `json:"result_bytes"`
}
```

`Session.Contexts []ContextStats` と非公開の `curContext int`（未開始は -1）を持つ。スライスへの append でポインタが無効になるため、カーソルはインデックスで保持する。

`BaselineTokens` は「その context の最初の assistant turn の `input + cache_creation + cache_read`」で、合計が 0 のターン（usage を持たない synthetic 等）は飛ばして最初の非ゼロを採る。実測は main context 中央値 49,674 / p90 58,788 / max 75,955（n=117）、subagent context 中央値 15,071（n=165）。

`report` 側は集計を別型で持つ。parser の型名と衝突させない。

```go
// ContextSummary is the per-context aggregate, split by origin because a
// subagent's context is not the main loop's.
type ContextSummary struct {
    Contexts         int `json:"contexts"`
    Turns            int `json:"turns"`
    BaselineMedian   int `json:"baseline_median_tokens"`
    BaselineP90      int `json:"baseline_p90_tokens"`
    BaselineMax      int `json:"baseline_max_tokens"`
    CacheReadPerTurn int `json:"cache_read_per_turn"`
}
```

`Summary` に `MainContexts ContextSummary` / `SubContexts ContextSummary` を足す。

```text
Contexts (one per transcript file)
  main       117 contexts   7469 turns   baseline median 49674   cache read 152327 / turn
  subagent   165 contexts   1864 turns   baseline median 15071   cache read  51027 / turn
  -> baseline is turn 1's whole prompt: system prompt + tool schemas + CLAUDE.md + skill list
```

この段階では baseline のシェアは出さない。baseline は毎ターン再読されるので、単純な `Σ baseline / Σ cache_read` は過小評価になる。シェアは Phase 4 の位置重み付けで初めて意味を持つ。

### Phase 4: 位置重み付き cache_read 帰属

turn t に注入された X トークンは、それ以降のターンで毎回 prompt に載る。したがって「何が cache_read を膨らませたか」は注入量ではなく注入量 × 残存ターン数で決まる。context ごとに compaction 境界で切ったセグメント（ターン数 T、index i）について次を積む。

```text
ctx_i    = input_i + cache_creation_i + cache_read_i
r_i      = T - i
i == 0   : baseline         += ctx_0 * r_0
i > 0    : delta_i           = ctx_i - ctx_(i-1) - output_(i-1)   (負は 0 にクランプ)
           assistant output += output_(i-1) * r_i
           by tool / other  += delta_i * r_i
```

`delta_i` の帰属先は「そのターンに届いた tool_result のツール」で、Phase 2 の join で一意に決まる（1 user 行に tool_result は 1 個だけであることを実測で確認済み）。tool_result を伴わないターンの delta は `other`（ユーザー入力、system-reminder、hook 出力）に落とす。

この式は恒等式になる。`Σ_i ctx_i`（context-turns）が全帰属の合計と一致するはずで、一致しない分はクランプした負 delta（context が縮んだターン）に由来する。実測では `Σ ctx_i` = 1,311,066,778 に対して帰属合計は 13.8% 過大で、負 delta は 607 ターン。この差を隠さず出す。

```go
// Attribution models which injection kept the context large, weighting every
// injection by the turns it stayed resident. The unit is token-turns, not
// tokens, and the total is checked against ContextTurns.
type Attribution struct {
    ContextTurns int            `json:"context_turns"`
    Baseline     int            `json:"baseline"`
    Output       int            `json:"assistant_output"`
    ByTool       map[string]int `json:"by_tool"`
    Other        int            `json:"other"`
    ClampedTurns int            `json:"clamped_turns"`
}
```

`Scope.Attribution Attribution` として持つ。context は main か sidechain のどちらか一方なので、context 単位で計算して該当スコープに積めばよい。`Merge()` に `mergeAttribution` を足す。

実装上、`r_i = T - i` は context（またはセグメント）の終端が確定しないと決まらないため、parser は turn の記録を一旦バッファする。`Session` に非公開の `curTurns []turnRecord` を持ち、`compact_boundary` を見た時点（`Compaction` を append する**前**）と `AppendReader` の終了時に flush する。バッファ量は turn 数ぶんの小さな struct で、実測 9,341 turn 全体でも無視できる。

出力は shares と over-attribution を並べる。

```text
Cache read attribution (position-weighted model; unit is token-turns)
  main   context-turns 1190412003
    baseline            36.5%
    assistant output    31.4%
    Read                10.3%
    Bash                 8.9%
    other injected       6.8%
    over-attribution   +13.8% (607 turns where the context shrank, clamped to 0)
  -> a model, not a measurement: deltas include system reminders arriving with the result
```

### Phase 5: skill 帰属と Read の作法

`rawLine` に 1 フィールド足す。未文書フィールドなので、無ければ空文字のまま何も起きない。

```go
AttributionSkill string `json:"attributionSkill"`
```

`Scope` に 3 つ足す。

```go
SkillTurns        map[string]int            `json:"skill_turns"`
SkillStarts       map[string]int            `json:"skill_starts"`
SubcommandBySkill map[string]map[string]int `json:"subcommand_by_skill"`
```

`SkillTurns` は skill 実行中の assistant turn 数（dedupe 後）。`SkillStarts` は値が直前の値と異なる非空値に変わった回数。`SubcommandBySkill` は skill → `"<host> <sub>"` の 2 段で、3 段のネストを避けるため値側のキーを平坦化する。skill 名が空のターンは `(none)` に寄せる。

実測では `Skill` tool_use 80 回（dev-workflow 49 / commit-and-draft-pr 21）に対し、`attributionSkill` の遷移は 141 回（dev-workflow 71 / commit-and-draft-pr 53）。現状の `SkillCounts` が skill 起動を 6 割ほど過小計測していたことになる。ただし遷移方式は、ネストした skill から親に戻る際に親を再度数える。distinct な (context, skill) の組は 126 で、差 15 が二重計上ぶん。この性質は README に書く。

I の Read 作法は `Scope` に 4 つ足す。

```go
ReadCalls      int            `json:"read_calls"`
ReadFullFile   int            `json:"read_full_file"`
ReadRepeat     int            `json:"read_repeat"`
ReadCounts     map[string]int `json:"read_counts"`
```

`ReadFullFile` は `input` に `offset` も `limit` も無い Read。判定は `json.RawMessage` を `map[string]json.RawMessage` に解いてキーの有無で見る（値 0 と未指定を区別する必要があるため、struct のゼロ値では判定できない）。実測 2,938 calls のうち 2,356（80.2%）が全文 Read。

`ReadRepeat` は「同一 context 内で 2 回目以降の同一 `file_path`」。判定単位を context にするのは、context が再読のコストが実際に発生する単位だから。レビューが挙げた 42.9% は論理セッション単位の数字で、値が変わることを README に書く。context ごとの既読集合は非公開フィールドで持ち、context 切り替え時にリセットする。

`ReadCounts` はパスを出す。既存の `FileCounts` が編集ファイルのパスを出しているのと同水準で、内容は出さない。

出力は degrade を明示する。

```text
Skills by attribution (attributionSkill; undocumented CLI field)
  dev-workflow          starts 71   turns 4676
  commit-and-draft-pr   starts 53   turns 1254
  -> Skill tool_use counted 80 invocations; nested starts are counted twice

Read discipline
  calls 2938   full-file (no offset/limit) 2356 (80.2%)   repeat within a context 1261 (42.9%)
```

`attributionSkill` を含む行が 1 つも無い場合は `(not recorded by this CLI version)` を出す。

## File changes

| パス                                                 | 変更内容                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `scripts/agent-stats/internal/parser/parser.go`      | Phase 1: エラー種別定数の入れ替えと `classifyToolError`。Phase 2: `rawContent.ID` / `ToolUseID`、`pendingCalls`、`ResultVolume`、`ToolErrorsByTool`、`toolResultSize`、`commandName` / `CommandSegments` / `Subcommand`、`SubcommandCounts`、`mergeVolumes` / `mergeNested`。Phase 3: `ContextStats`、`Session.Contexts`、`AppendReader` シグネチャ、baseline。Phase 4: `Attribution`、turn バッファと flush。Phase 5: `AttributionSkill`、skill 集計、Read 集計 |
| `scripts/agent-stats/internal/parser/parser_test.go` | 各フェーズのテスト追加（下記テスト戦略）                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `scripts/agent-stats/internal/report/report.go`      | Phase 1: `TurnStats`、`ActiveCoverage`、見出し 2 行化、top sessions の median 列。Phase 2: `Breakdown` / `rankedNested`、出力量セクション、サブコマンドセクション、エラー種別の幅計算。Phase 3: `ContextSummary` と Contexts セクション。Phase 4: 帰属セクション。Phase 5: skill / Read セクション                                                                                                                                                               |
| `scripts/agent-stats/internal/report/report_test.go` | 新スキーマ追随とテスト追加                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `scripts/agent-stats/cmd/agent-stats/main.go`        | `parser.AppendFile` 経由なので変更不要。`AppendReader` を直接呼んでいないことを確認する                                                                                                                                                                                                                                                                                                                                                                          |
| `scripts/agent-stats/README.md`                      | 新指標の定義と限界（下記 README 節）                                                                                                                                                                                                                                                                                                                                                                                                                             |

コミット単位はフェーズごとに 1〜2 本。Phase 1 は parser（分類）と report（Active time）で 2 本に割る。Phase 2 は join / 出力量で 1 本、サブコマンド分解で 1 本。Phase 3 / 4 / 5 は各 1 本。parser と report のスキーマが同時に動くフェーズでは中間状態をコミットしない。

## Risks and mitigations

- **エラー分類の文言マッチが CLI 更新で壊れる。** 分類不能は `ErrFailure` に落ち総数は常に正確、という現行の性質を維持する。判定順（hook 最優先）をテストで固定し、hook メッセージが拒否文言を含んでも hook に落ちることを保証する。
- **差分法が over-attribution する。** 実測 13.8%。負 delta（context の縮小）をクランプした結果であり、原因は microcompaction や tool_result の切り詰めと推測される。恒等式との差とクランプ件数を必ず出力し、シェアを 100% に正規化しない。
- **`attributionSkill` が消える。** 未文書フィールド。欠落時は全マップが空のまま「記録なし」と表示する degrade テストを置く。
- **`AppendReader` のシグネチャ変更。** `internal` パッケージなので影響は 3 箇所 + テスト。コンパイルエラーで確実に検出される。
- **サブコマンド分解の誤検出。** allowlist と `^[a-z][a-z0-9-]*$` で抑えるが、`git -C <相対パス> status` は誤る。実測での出現は 0。`BashCounts` は先頭方式のまま残すので、既存の読み方は変わらない。
- **同一 `message.id` が 2 ファイルに現れると context の turn が 0 になる。** 前サイクルの実測で 7,778 件中 1 件のみ。context 単位の turn 数がごく稀に 1 少なくなる既知の限界として README に書く。
- **`--json` が肥大する。** context は 297 件、`contexts` 配列はその規模。`list` と同じ扱い（`--detail` 限定）にするか常時出すかは Open questions。
- **出力が長くなる。** table に 5 セクション増える。1 人用のローカル CLI なので常時表示を既定とするが、`--json` での消費を前提に節ごとの見出しを崩さない。

## Validation

各フェーズ完了時に通す。

```sh
mise run agent-stats:test
mise run agent-stats:lint
```

実データでの動作確認（read-only なので安全）。

```sh
cd scripts/agent-stats
go run ./cmd/agent-stats
go run ./cmd/agent-stats --json | jq '.tool_result_volume, .main_contexts, .sub_contexts'
go run ./cmd/agent-stats --json | jq '.main.attribution, .subcommand_counts'
go run ./cmd/agent-stats --json --detail | jq '.list[0].contexts'
```

回帰確認は、同じ transcript に対して独立に書いたスクリプトの値と突き合わせる。transcript は増えるので絶対値は固定できない。以下は 2026-08-28 時点（297 ファイル）の独立計算値で、実装後に一致すべき対象。

| 項目                         | 独立計算値                                                                                                      |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `tool_result` 総数           | 11,719（Bash 6,029 = 51.4% / Read 2,940 = 25.1%）                                                               |
| `is_error` 総数              | 405                                                                                                             |
| 新分類                       | hook 8 / user_reject 42 / permission_denied 44 / invalid_input 8 / failure 303                                  |
| user_reject の内訳           | ExitPlanMode 31 / AskUserQuestion 8 / Bash 3                                                                    |
| permission_denied の内訳     | Bash 32 / Read 10 / Agent 2                                                                                     |
| tool_result 文字数           | 計 28,547,116（Read 52.1% / Bash 35.3%、avg 5,065 / 1,684）                                                     |
| baseline（main context）     | median 49,674 / p90 58,788 / max 75,955（n=117）                                                                |
| baseline（subagent context） | median 15,071（n=165）                                                                                          |
| cache_read / turn            | main 152,327（7,469 turns）/ sub 51,027（1,864 turns）                                                          |
| context-turns 合計           | 1,311,066,778                                                                                                   |
| 帰属シェア                   | baseline 36.5% / assistant output 31.4% / tool 経由 39.2% / other 6.8% / over-attribution +13.8%（clamped 607） |
| git サブコマンド             | status 190→422 / commit 100→139 / push 118→123                                                                  |
| Read                         | 2,938 calls / 全文 2,356（80.2%）                                                                               |
| skill                        | `Skill` tool_use 80 / `attributionSkill` 遷移 141 / distinct (file, skill) 126                                  |
| turn_duration                | 512 / 9,341 = 5.5%、sum 168h23m、median 1m33s、p90 14m2s、p95 23m11s、max 88h35m、上位 5 で 74.6%               |

## Tests

`internal/parser/parser_test.go` はインライン JSONL を `ParseReader` に流す既存パターンと、純粋関数はテーブル駆動という既存パターンを踏襲する。`t.Parallel()` は関数とサブテストの両方に付ける。

- `TestClassifyToolError` — テーブル駆動。`Permission to use Bash with command ls has been denied.` → permission_denied、`Permission for this action was denied by the Claude Code auto mode classifier. Reason: ...` → permission_denied、`<tool_use_error>File is in a directory that is denied by your permission settings` → permission_denied、`The user doesn't want to proceed with this tool use. The tool use was rejected.` → user_reject、`PreToolUse:Bash ...` → hook、`PreToolUse:Bash ... has been denied` → hook（判定順ガード）、`<tool_use_error>InputValidationError: Read was called with ...` → invalid_input、`Exit code 1\nno such file` → failure、`""` → failure、未知文言 → failure
- `TestParseReaderToolErrors` — 新バケツへの追随と `ToolErrorsByTool` の cross-tab
- `TestParseReaderToolResultVolume` — id つき tool_use と `tool_use_id` つき tool_result の join。Read / Bash のバイト数、`BashResultVolume` が `LeadingCommand` キーで積まれること、content が blob 配列のとき text のみ合算されること（`tool_reference` は 0）
- `TestParseReaderToolResultVolumeWithoutIDs` — `id` / `tool_use_id` の無い旧形式で `(unknown)` に寄り、`ToolResults` 総数は変わらないこと
- `TestCommandSegments` — テーブル駆動。`git add x && git commit -m y` → 2 セグメント、`git status | head -20` → status のみ、`cd /a && git push` → push + withCd、`cat <<'EOF' > f\ngit commit -m nope\nEOF` → 記録なし、`git commit -m "fix: a|b"` → commit 1（引用符内分割の無害化）、`git -C /x status` → status、`mise run lint && go test ./...` → mise/run と go/test、`cat foo.txt` → 記録なし（allowlist 外）
- `TestLeadingCommand` — 無変更で通ること（`commandName` 抽出の回帰ガード）
- `TestParseReaderSubcommands` — fixture レベルでの `SubcommandCounts`
- `TestParseReaderContexts` — 1 Session に 2 回 `AppendReader` して context が 2 件、sidechain フラグ、compaction 件数、baseline がそれぞれの 1 turn 目から採られること
- `TestParseReaderBaselineSkipsZeroUsage` — usage の無い先頭ターンで baseline が 0 のままにならないこと
- `TestParseReaderAttribution` — 手計算できる 3 ターン fixture で baseline / output / by_tool / context_turns を検証。中途に `compact_boundary` を挟んで残存ターンがリセットされること。context が縮むターンで `ClampedTurns` が増え、帰属が負にならないこと
- `TestParseReaderAttributionSkill` — dev-workflow ×3 → commit-and-draft-pr ×2 → dev-workflow ×1 で `SkillTurns` 4/2、`SkillStarts` dev-workflow 2 / commit 1（ネスト復帰の二重計上を仕様として固定）
- `TestParseReaderWithoutAttributionSkill` — フィールドが 1 行も無い fixture で全マップが空、panic なし
- `TestParseReaderReadDiscipline` — offset / limit あり / なし、同一 context 内の 2 回目で `ReadRepeat` が 1、別 context の同一パスは repeat にならないこと
- `TestScopeMergeIntoZeroValue` — 新フィールド（volume / nested / attribution）を含めて zero-value への 2 回 Merge で倍になること
- `TestMergeNested` と `TestMergeVolumes` — ヘルパの単体テスト

`internal/report/report_test.go` に追加する。

- `TestTurnStats` — テーブル駆動。空 / 1 件 / 3 件 / 20 件で median・p90・p95・longest のインデックスを固定
- `TestSummarizeActiveCoverage` — カバレッジ 50% 未満で警告行が出る、50% 以上で出ないこと
- `TestRenderTableToolOutputVolume` — 回数シェアとサイズシェアの両方が出ること
- `TestSummarizeContexts` — main / sub の context 数と baseline median / p90
- `TestSummarizeAttribution` — セッション横断の畳み込みと over-attribution の表示
- `TestRankedNested` — 並び順（total desc → name asc）と limit
- `TestRenderTableDegradesWithoutSkillAttribution` — `(not recorded ...)` が出ること
- `TestRenderTable` / `TestRenderJSONRoundTrip` — 既存テストの新スキーマ追随

## README changes

`scripts/agent-stats/README.md` は指標の定義と既知の限界を丁寧に書く構成を保つ。

- 「何を集計するか」に追記: tool 別の tool_result 出力量、context 単位の集計と baseline、cache_read の位置重み付き帰属、サブコマンド内訳、`attributionSkill` による skill 帰属、Read の作法。
- 「時間の 2 つの指標」を改訂: カバレッジ率の併記、p90 / p95 / max の並記、外れ値を除外しない方針とその理由、`Top sessions` の `active` 列が median であること。
- 「集計の読み方の注意」に新項目:
  - エラー分類の 5 バケツ定義。`user_reject` は摩擦ではなく人間の意思表示（実測で ExitPlanMode と AskUserQuestion が大半）であり、`permission_denied`（権限システムによる拒否）と混ぜて読まないこと。
  - tool_result の出力量は **chars** であってトークンではない。回数シェアとサイズシェアは逆転する。
  - cache_read の帰属は差分法による**モデル**。単位は token-turn。恒等式との差（over-attribution）とクランプ件数を併記しており、100% に正規化していない。system-reminder や hook 出力が tool_result と同じターンに届くため、その分がツールに寄る。
  - baseline は context の 1 turn 目の prompt 全長。main と subagent で桁が違う。
  - context は 1 transcript ファイル。subagent の tool_result は親の cache_read を増やさない。
  - サブコマンド分解は allowlist 方式で、引用符内の `|` と `git -C <相対パス>` に既知の限界がある。`Bash breakdown`（先頭方式）は互換のため残しており、両者の合計は一致しない。
  - `attributionSkill` は未文書フィールド。CLI 2.1.218〜2.1.248 で確認。無ければ静かに degrade する。skill start は遷移数なのでネスト復帰を二重に数える。
  - Read の重複は context 単位の判定であり、論理セッション単位で数えると値が変わる。
- 「設計・制約」のプライバシー節に追記: Read されたファイルパスも出力する（既存の編集ファイルパスと同水準、内容は出さない）。
- 「使い方」の `jq` 例を新フィールドに更新。
- `--json` の後方互換方針を 1 段落で明記（下記）。

## Backward compatibility

`--json` を消費する下流は存在しない。`grep -rn "agent-stats"` の結果は `mise.toml` のタスク、`ci_agent_stats.yml`、`AGENTS.md` の説明、`docs/plan/**` のみ。よって破壊的変更のコストはほぼ無いが、無闇に壊す理由も無い。方針は「意味が変わらない名前は残し、名前が嘘になる箇所だけ直す」。

- 追加のみ: `tool_result_volume`、`bash_result_volume`、`tool_errors_by_tool`、`subcommand_counts`、`contexts`、`main_contexts`、`sub_contexts`、`attribution`、`skill_turns`、`skill_starts`、`subcommand_by_skill`、`read_*`、`p90_turn_ns`、`p95_turn_ns`、`active_coverage`、`active_median_ns`。
- 値レベルの変更: `tool_errors` のバケツ名（`permission` → `user_reject` / `permission_denied` / `invalid_input`）。これは修正対象そのものなので変える。README に旧名との対応を書く。
- 残す: `median_turn_ns` / `longest_turn_ns` / `active_duration_ns` / `bash_counts` / `skill_counts` / `tool_counts`。特に `bash_counts`（先頭方式）は `subcommand_counts` と併存させる。両者は別の問いに答えるため。
- 削除: `parser.ErrPermission` 定数（`internal` パッケージの API なので影響は同一モジュール内のみ）。
- table 出力はスキーマではないため、`Top sessions` の `active` 列を合計から median に差し替える。

## Open questions

- **`auto mode classifier` の拒否を `permission_denied` に畳むか分けるか。** 実測 3 件（Agent 2 / Bash 1）。打ち手は auto mode の設定であり allowlist とは別物だが、母数が小さい。本計画では畳んで README に注記する案を採る。分けるなら定数 1 つとテスト 1 ケースの追加で済む。
- **skill start の定義。** 遷移 141 と distinct (context, skill) 126 のどちらを「起動数」と呼ぶか。本計画は遷移を採り、二重計上を README に書く方針。distinct を併記する価値があるかは実装後に判断する。
- **帰属セクションを常時出すかフラグにするか。** 出力が 20 行前後増える。計算コストは無視できる。
- **`contexts` 配列を `--json` に常時含めるか。** 297 件規模。`list` と同じく `--detail` 限定にする案がある。
- **サブコマンド allowlist の範囲。** 現案は 14 コマンド。`sed` / `jq` のようにサブコマンドを持たないが引数パターンが意味を持つものをどう扱うかは未決。
- **over-attribution 13.8% の原因を追うか。** 負 delta 607 ターンの内訳（microcompaction、tool_result の切り詰め、thinking の破棄）を特定すればモデルの精度は上がる。本計画では数値の開示までとする。
- **Read 重複の判定単位。** context 単位（本計画）と論理セッション単位で値が変わる。両方出すかは実装後に判断する。
