# Plan: 変更行のpatch coverageを算出する `cover-diff` を `scripts/` に追加する

GitHub issue [#673](https://github.com/hodanov/my-pde/issues/673) の採用が決まった。`scripts/`配下のGoモジュールについて、「差分で追加・変更した行のうちテストが1度も実行していない行」を行番号つきで出すread-only CLI `cover-diff` を新規Goモジュール `scripts/cover-diff/` として追加する。cloud routine（`weekly-adopted-issue-pr-bot`）で実装するには重いと判断し、ローカル開発環境で実装した。

## Background

`scripts/go-verify`はCIと同じ手順（goimports / golangci-lint / go test）をローカルで回すpass/failゲートだが、`-coverprofile`は無い。カバレッジを取っているのは`.github/workflows/ci-go-module.yml`だけで、`go tool cover -func`によるモジュール全体の総%と全関数一覧をPRにsticky commentで貼るのみ。差分とは紐づいておらず、`coverage.out`はジョブ終了時に破棄される。

このリポジトリはRoutine（`weekly-adopted-issue-pr-bot`）がPRの大半を自動生成し、人間側の作業は実質レビューだけという運用になっている。「変更行のうち未テストの行」はレビュー時にどこを重点的に見るかを決める安いシグナルになる、というのがissueの動機。

## Current structure

- `scripts/`配下の既存Goモジュール: `agent-stats` / `ai-bridge` / `config-diff` / `go-verify` / `nvim-sync` / `pipeline-metrics` / `scaffold`。すべてmodule名がディレクトリ名と一致し、`ai-bridge`と`nvim-sync`以外は標準ライブラリのみに依存する
- `scripts/pipeline-metrics`の設計の前提: 外部コマンドを呼ばない、入力はファイル/stdinで注入できる、一切書き込まない、`testdata/`の固定入力に対するgolden testで出力をバイト単位固定する
- `scripts/go-verify`の`internal/runner`: 外部コマンド実行を`type Runner func(dir, name string, args ...string) ([]byte, error)`として注入し、`cmd/go-verify/main.go`の`defaultRunner`だけが`exec.Command`を触る
- `scripts/scaffold`: `scaffold new <name> [--from <module>]`で骨格と`.github/workflows/ci-<name>.yml`を生成し、`mise.toml`に貼るタスクブロックを標準出力に出す
- `verify-changed.sh` / `test-changed.sh`は`scripts/*/*.go`をディレクトリ名でモジュール判定する。`case`の`*`は`/`にもマッチするため`internal/**`配下も拾う（実測確認済み）ので、`scripts/cover-diff/`は追加するだけで自動的に対象になる

## Design policy

### 配置とモジュール構成

net-new Goモジュール `scripts/cover-diff/`。Go 1.26、外部依存ゼロ。骨格は`scaffold new cover-diff --from pipeline-metrics`で生成した。`--from`に`pipeline-metrics`を選んだのは、そのモジュールが持つ`<name>:run`タスクがそのまま引き継がれ、issue本文が求める人間の入口`mise run cover-diff:run`を追加作業なしで得られるため。

```text
scripts/cover-diff/
  cmd/cover-diff/main.go  # フラグ解釈 → 収集 → 解析 → 出力、終了コード
  internal/udiff/         # unified diffパーサ（追加行の行番号集合をファイル別に返す）
  internal/cover/         # cover profileパーサ（ブロック -> 行 -> covered/uncovered）
  internal/patch/         # 座標系変換・グルーピング・除外・突き合わせと、唯一のexec境界
  internal/report/        # text / json レンダラ
  testdata/               # 固定のdiff + profileと golden 出力
```

`internal/udiff`と`internal/cover`は純粋パーサで、gitもgoも呼ばない。exec は`internal/patch`の`Runner`経由に限り、`go-verify`と同じ注入パターンを取る。issue本文の3パッケージ構成に`internal/patch`を足したのは、`go-verify`（`internal/runner`）や`pipeline-metrics`（`internal/metrics`）と同じく結合ロジックを`cmd/*/main.go`に置かないため。

### CLI仕様

```text
cover-diff [--root <dir>] [--base origin/main] [--mod <substr>]
           [--diff <file>|-] [--profile <module>=<path>] [--exclude <regexp>]
           [--coverpkg] [--threshold <pct>] [--format text|json]
```

処理: (1) unified diffから新しい側の行番号集合をファイル別に抽出（`_test.go`は常時除外） → (2) `scripts/<app>/`の先頭要素でモジュールに割り当て → (3) `--profile`指定が無ければ該当モジュールでのみ`go test ./... -count=1 -coverprofile=<tmp>`を実行 → (4) プロファイルのブロックを行に展開し`covered`/`uncovered`/ステートメント無しの3値に分類 → (5) モジュール別サマリ＋未カバー行のレンジ＋全体patch coverageを出力。

### 実装時に確定した設計判断

issue本文と初版プランで未確定だった論点を、調査結果をもとに次のとおり決めた。

- **rootの解決**: `--root`未指定なら`git rev-parse --show-toplevel`で求める。既存のmiseタスクはすべて`dir = "scripts/<app>"`を持つため`mise run cover-diff:run`のcwdは`scripts/cover-diff`になる。既定モードではどのみちgitを叩くので依存は増えず、フラグ無しで動く入口が手に入る。`--diff`/`--profile`注入時はrootを一切解決しない。
- **測定不能時の終了コード**: `--threshold`指定時にプロファイルを取れなければ`2`。閾値を要求したのに測れなかったのは保証できないということなので、`0`で静かに通さない。閾値割れの`1`とも区別する。
- **`--exclude`の既定値**: 空。常時除外は`_test.go`のみ。CIは`ci-ai-bridge.yml`に`test-exclude-pattern: "/mock$|/cmd/"`をハードコードしているが、これはai-bridge固有の設定であり全モジュールへ効かせる理由が無い。`pipeline-metrics`/`go-verify`の非ハードコード流儀に従い、READMEで`--exclude '/mock/|/cmd/'`を例示するに留めた。
- **AI呼び出し配線は本PRに含めない**。`ai-agents/agents/`のうちBashを持つのは`code-review-scanner`と`verify-runner`だけで、4観点エージェントと`code-review-critic`は`Read, Grep, Glob`のみで実行手段が無い。配線はCLIの上に乗る利用チャネル追加という別種の変更であり、CLI本体の正しさ（パーサのリスクだけで複数項目ある）と切り離した方がレビュー可能な単位を保てる。実装完了後に別issueを起票する。
- **testdataは合成テキストのみ**。実ビルド可能なGoソースと実`go test`出力は使わず、手で書いたdiff/profileで固定する。ただしプロファイルの書式は実測に合わせた（下記）。

### 調査で判明した設計上の穴: 座標系の不一致

初版プランに記述が無く、放置すれば突き合わせが全滅する静かな失敗になっていた点。実測すると、cover profileのパスは**import path**（`config-diff/cmd/config-diff/main.go`）である一方、`git diff`のパスは**リポジトリ相対**（`scripts/config-diff/cmd/config-diff/main.go`）だった。

このリポジトリは全7モジュールでmodule名がディレクトリ名と一致するため、プロファイルパスの第1セグメントを剥がして`scripts/<module>/`を前置すれば変換できる。第1セグメントがモジュール名と一致しないパス（vendorされた依存など）は警告に積んで読み飛ばす。この規約への依存はREADMEの「制約」に明記した。

### PDEとの連携

- **人間の入口**: `mise run cover-diff:run`
- **AI（自動ゲート側）**: `verify-changed.sh`（Stopフックのゲート）には組み込まない。重い処理を足すとStopフックの体感が落ちるため
- **AI（レビュー側）**: 「必要なときに明示的に呼ぶread-only道具」として置く。`--format json`は将来digest/レビューBotが読める形にしてある
- CIへの組み込み（coverage.outのartifact化、PRコメント差し替え、閾値fail）はissue本文が明示的に別issueへスコープを切っている。本プランもそれに倣い、ローカルCLIまでに閉じた

## Implementation steps

1. `scaffold new cover-diff --from pipeline-metrics --root ../..`で骨格生成。出力された`mise.toml`ブロックを`# ---- Aggregates ----`の直前へ貼り、`go:test`/`go:lint`の`depends`に`cover-diff:test`/`cover-diff:lint`を追加。`.gitignore`（`/cover-diff`と`coverage.out`）はscaffoldが生成しないので手で足す
2. `internal/udiff`を実装。依存の無い最下層のため最初に固める。hunkヘッダが宣言する新旧の行数を数え、その本文を消費し切るまでヘッダ判定に戻らないことで、`+++`や`@@`で始まる**追加されたソース行**をヘッダと誤認しないようにする
3. `internal/cover`を実装。1行目`mode:`の検証と、「ステートメントが無い行をmapのキーに出さない」ことをここで固定する。ブロックの終端位置は排他なので、`eL.1`で終わるブロックは`eL`を含めない（閉じ括弧が分母に入らない）
4. `internal/patch`を実装。座標系変換、モジュールグルーピング、`_test.go`/`--exclude`適用、3値分類、`Runner`を取る収集関数（`RepoRoot`/`Diff`/`WriteProfile`）
5. `internal/report`を実装。text / json レンダラ
6. `cmd/cover-diff/main.go`を組み立てる。`execute(args, run, stdin, out) int`で終了コードを返しつつ`Runner`を注入する（`go-verify`のハイブリッド型）
7. `testdata/`とgolden testを整備
8. `scripts/cover-diff/README.md`を作成
9. `AGENTS.md`のモジュール一覧2箇所に`cover-diff`を追加
10. 実リポジトリの過去コミット範囲に対して実運用コマンドで動かし、出力を目視確認する

## File changes

- 新規: `scripts/cover-diff/{go.mod, .gitignore, README.md, cmd/cover-diff/, internal/{udiff,cover,patch,report}/, testdata/}`
- 新規（scaffold生成）: `.github/workflows/ci-cover-diff.yml`
- 編集: `mise.toml`（`cover-diff:*`タスクブロック追加、`go:test`/`go:lint`の`depends`追加）
- 編集: `AGENTS.md`（モジュール一覧2箇所）
- 変更しないもの: `.github/workflows/ci-go-module.yml`（既存7モジュールのCI挙動を一切動かさない）、`ai-agents/scripts/verify-changed.sh` / `.claude/hooks/test-changed.sh`（汎用パターンで自動追従、編集不要）、`ai-agents/agents/*.md`（別issueへ）

## Risks and mitigations

1. **【最重要】ステートメントが無い行の判定を誤ると全部ノイズになる。** import/構造体宣言/コメント/閉じ括弧はcover profileのどのブロックにも属さない。→ `internal/cover`が「ブロックが張っていない行をmapのキーに出さない」設計にし、呼び出し側は「mapに無い行=分母外」として扱う。golden testの最初のケースとして固定した
2. **座標系の取り違え。** プロファイルはimport path、diffはリポジトリ相対。→ 変換を`internal/patch`の1関数に閉じ、モジュール名と一致しないパスは警告に出す
3. **package単位カバレッジの死角。** `go test ./...`の既定では各packageは自packageのテストにしか計測されない。→ 既定はCIと同じ（`-coverpkg`なし）に揃え、`--coverpkg`をオプトインで用意し、READMEに死角を明記した
4. **テスト再実行コスト。** 既定動作は`go test -coverprofile`を走らせるためgo-verifyやStopフックと二重に回る。→ `--mod`で絞る/`--profile`で既存プロファイルを再利用する逃げ道を用意し、Stopフックには入れない
5. **生成コードの扱い。** `ai-bridge`の`mock_port.go`等。→ `--exclude`を用意し、READMEでmockパスを例示した
6. **diffの取り方の穴。** rename/mode変更のみのhunk、`\ No newline at end of file`、壊れたhunkヘッダ、`+++`で始まる追加行、日本語ファイル名のquote。→ 解釈できないhunkは`Warnings`に積んで全体をfailさせない。gitは`-c core.quotePath=false`で起動し、それでもquoteされたパスは`strconv.Unquote`で復元する
7. **CI検証への影響。** net-newモジュールなので既存7モジュールのCI挙動は変わらない。`ci-go-module.yml`自体は変更しない
8. **AI呼び出し配線を今回含めないことによる「絵に描いた餅」状態の継続。** → 実装完了後に速やかに別issueを起票する運用でカバーする

## Validation

1. `internal/udiff`・`internal/cover`・`internal/patch`・`internal/report`はtable-driven test（`reflect.DeepEqual`での構造体全体比較）で個々の分岐を潰す
2. `cmd/cover-diff`配下でend-to-endのgolden testを固定する（`pipeline-metrics`の`-update`フラグ運用を流用）。`testdata/sample.diff`には次を1本に混在させる: ステートメントの無い変更行、rename-only/mode-onlyのブロック、no-newlineマーカー、`+++ /dev/null`の削除ファイル、壊れたhunkヘッダ、`_test.go`、生成コードパス、複数モジュール、`scripts/`外のファイル
3. golden testに渡す`Runner`は呼ばれたら`t.Errorf`する実装にし、フィクスチャ経路が実プロセスを起動しないことを構造的に保証する
4. `--format json`のフィールド名を固定する契約テストと、同一入力で5回実行してバイト一致を確認する決定論テスト
5. `--threshold`のON/OFFと測定不能ケースの終了コード（0/1/2）を`execute`の戻り値で直接assertする
6. `mise run cover-diff:test` / `cover-diff:lint` / `cover-diff:build`をローカルで通す
7. 実リポジトリの過去コミット範囲に対して実運用コマンドで動かし、報告された未カバー行が実際にステートメント行であること（閉じ括弧やコメントが混じっていないこと）をソースと突き合わせて確認する
8. `mise run verify:changed`がcover-diffの変更を自動でpickupすることを確認する

## Out of scope

- CIへの組み込み（`coverage.out`のartifact化、PRコメントをpatch coverageに差し替え、閾値fail）。`ci-go-module.yml`は全モジュール共有なので影響範囲もレビュー観点も別物
- `ai-agents/agents/code-review-scanner.md`への`cover-diff`実行手順の追記
