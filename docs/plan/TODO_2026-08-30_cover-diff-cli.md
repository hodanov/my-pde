# Plan: 変更行のpatch coverageを算出する `cover-diff` を `scripts/` に追加する

GitHub issue [#673](https://github.com/hodanov/my-pde/issues/673) の採用が決まった。`scripts/`配下のGoモジュールについて、「差分で追加・変更した行のうちテストが1度も実行していない行」を行番号つきで出すread-only CLI `cover-diff` を新規Goモジュール `scripts/cover-diff/` として追加する。cloud routine（`weekly-adopted-issue-pr-bot`）で実装するには重いと判断し、ローカル開発環境で実装する。

## Background

`scripts/go-verify`はCIと同じ手順（goimports / golangci-lint / go test）をローカルで回すpass/failゲートだが、`-coverprofile`は無い。カバレッジを取っているのは`.github/workflows/go_module_ci.yml`だけで、`go tool cover -func`によるモジュール全体の総%と全関数一覧をPRにsticky commentで貼るのみ。差分とは紐づいておらず、`coverage.out`はジョブ終了時に破棄される。

このリポジトリはRoutine（`weekly-adopted-issue-pr-bot`）がPRの大半を自動生成し、人間側の作業は実質レビューだけという運用になっている。「変更行のうち未テストの行」はレビュー時にどこを重点的に見るかを決める安いシグナルになる、というのがissueの動機。

**採用判断の過程で見つけた追加の論点**: issue本文は「レビュー用サブエージェントが必要なときに明示的に呼ぶ」と設計しているが、実際に確認すると`ai-agents/agents/review-correctness.md`・`code-review-critic.md`は`tools: Read, Grep, Glob`のみでBashを持たず、cover-diffを実行する手段が無い。`code-review-scanner.md`はBashを持つが「Test coverage」節は目視判断の指示のみで、外部コマンドを叩く指示は無い。この配線をどう扱うかは本プランのDesign policyで選択肢を示し、Open questionsで確定を求める。

## Current structure

- `scripts/`配下の既存Goモジュール: `ai-bridge` / `pipeline-metrics` / `go-verify` / `config-diff` / `nvim-sync` / `scaffold` / `agent-stats`
- `scripts/pipeline-metrics`の設計の前提（`README.md`実測済み）: 外部コマンド（`gh`等）を呼ばない、入力はファイル/stdinで注入できる、一切書き込まない、`testdata/`の固定入力に対するgolden testで出力をバイト単位固定する
- `scripts/scaffold`: `scaffold new <name> [--from <module>]`で新規Goモジュールの骨格（`go.mod` / `cmd/<name>/main.go` / `README.md`）と対になるCIワークフロー（`.github/workflows/ci_<name>.yml`）を生成し、`mise.toml`に貼るタスクブロックを標準出力に出す。`--from`は元モジュールの`mise.toml`セクションをトークン置換して複製する（`internal/gen/gen.go`の`extractMiseSection`）
- `ai-agents/agents/`のレビューエージェント: `review-correctness.md`/`code-review-critic.md`はRead/Grep/Globのみ、`code-review-scanner.md`のみBashを持つがcover-diffのようなCLI実行の指示は無い
- `verify-changed.sh` / `test-changed.sh`は`scripts/*/*.go`をディレクトリ名でモジュール判定する汎用パターンなので、`scripts/cover-diff/`は追加すれば自動的に対象になる（スクリプト側の編集は不要）

## Design policy

### 配置とモジュール構成

net-new Goモジュール `scripts/cover-diff/`。Go 1.26、外部依存ゼロ（標準ライブラリのみ）。骨格は`scaffold new cover-diff --from pipeline-metrics`で生成する。`--from`に`pipeline-metrics`を選ぶ理由は、そのモジュールが持つ`<name>:run`タスク（`go run ./cmd/<name>`）がそのまま引き継がれ、issue本文が求める人間の入口`mise run cover-diff:run`を追加作業なしで得られるため（既定の`config-diff`には`:run`タスクが無い）。

```text
scripts/cover-diff/
  cmd/cover-diff/main.go
  internal/udiff/     # unified diffパーサ（追加行の行番号集合をファイル別に返す）
  internal/cover/     # cover profileパーサ（ブロック -> 行 -> covered/uncovered/no-stmt）
  internal/patch/     # 突き合わせ・グルーピング・除外の結合ロジック（本プランでの追加提案）
  internal/report/    # text / json レンダラ
  testdata/           # 固定のdiff + profileと golden 出力
```

`internal/patch`はissue本文の3パッケージ構成（udiff/cover/report）に対する追加提案。`go-verify`（`internal/runner`）や`pipeline-metrics`（`internal/metrics`）が「ドメインロジックはcmd/main.goに置かず、exec境界に依存しない純粋関数としてinternalに置く」一貫したパターンを取っているのに合わせ、diffの追加行×coverプロファイル×モジュール名グルーピング×`--exclude`適用という結合ロジックをmain.goから切り出す。採否はOpen questions参照。

### CLI仕様

```text
cover-diff [--root .] [--base origin/main] [--mod <substr>]
           [--diff <file>|-] [--profile <module>=<path>]
           [--exclude <regexp>] [--threshold <pct>] [--format text|json]
```

処理: (1) unified diffから追加・変更後の行番号集合をファイル別に抽出（`_test.go`は既定除外） → (2) `scripts/<app>/...`の先頭要素でモジュールに割り当て → (3) `--profile`指定が無ければ該当モジュールでのみ`go test ./... -coverprofile=<tmp>`を実行 → (4) プロファイルのブロックを行に展開し`covered`/`uncovered`/`no-stmt`の3値に分類（分母は`no-stmt`を除くステートメント行のみ） → (5) モジュール別サマリ＋未カバー行のレンジ＋全体patch coverageを出力。`--threshold`指定時のみ下回ったら非ゼロ終了（既定は常に0終了の情報提供ツール）。

### PDEとの連携

- **人間の入口**: `mise run cover-diff:run`
- **AI（自動ゲート側）**: `verify-changed.sh`（Stopフックのゲート）には組み込まない。重い処理を足すとStopフックの体感が落ちるため
- **AI（レビュー側）**: 「必要なときに明示的に呼ぶread-only道具」として置く。ただし既存レビューエージェントへの配線は本プランのスコープに含めない（下記参照）。`--format json`は持たせておき、将来digest/レビューBotが読める形にしておく
- CIへの組み込み（coverage.outのartifact化、PRコメント差し替え、閾値fail）はissue本文が明示的に別issueへスコープを切っている。本プランもそれに倣い、ローカルCLIまでに閉じる

### AI呼び出し配線（採用判断時に見つけたギャップへの対応）

**選択肢B（今回は含めず、別issueに切り出す）を採る。** 理由: (1) issue #673自体が「CIへの組み込み」を別issueへスコープを切っているのと同じ理屈で、配線もCLIの上に乗る利用チャネル追加という別種の変更であり、CLI本体の正しさ（パーサのリスクだけで8項目ある）と切り離した方がレビュー可能な単位を保てる。(2) 配線には「常時走らせるか条件付きか」「package単位カバレッジの死角をプロンプトでどう説明するか」など、CLIが実際に動いてから決めた方がいい設計判断が残っている。

本実装のPRは`scripts/cover-diff`のみに留め、完了後に「`code-review-scanner.md`のTest coverage節に、`scripts/*/**.go`の変更がある場合のみ`cover-diff`をBashで実行し未カバー行をScan Reportへ反映する手順を追記する」という内容で別issueを起票する。

## Implementation steps

1. `scaffold new cover-diff --from pipeline-metrics --root .`で骨格生成。出力された`mise.toml`ブロックを貼り、`go:test`/`go:lint`の`depends`に`cover-diff:test`/`cover-diff:lint`を追加する
2. `internal/udiff`（unified diffパーサ）を実装。依存の無い最下層のため最初に固める。`testdata`内の固定diffを使ったtable-driven testを同時に書く
3. `internal/cover`（cover profileパーサ）を実装。「no-stmt行を分母から除外する」「1行目`mode:`の検証」をここで固定する
4. `internal/patch`（突き合わせ・グルーピング・`--exclude`適用）を実装。`scripts/<mod>/...`グルーピングは`verify-changed.sh`/`test-changed.sh`と同じ規約をGoで再実装する
5. `internal/report`（text / json レンダラ）を実装。`pipeline-metrics/internal/render`と同型の純粋関数
6. `cmd/cover-diff/main.go`を組み立てる。`pipeline-metrics/cmd/pipeline-metrics/main.go`の`run(args, stdin, out)`パターンを踏襲し、golden testでは実プロセス（git/go test）を一切起動しない
7. `testdata/`とgolden testを整備（詳細はValidation参照）
8. `scripts/cover-diff/README.md`を作成。設計の前提、package単位カバレッジの死角、`--exclude`の使用例（mockパス）、終了コード表を明記する
9. `mise.toml`の`:run`タスク説明文を実用途に書き換え、`cover-diff:test`/`:lint`/`:build`をローカルで通す。`.github/workflows/ci_cover_diff.yml`が`go_module_ci.yml`を`module: cover-diff`で呼ぶだけになっていることを確認
10. dev-workflowのverifyフェーズとして、実リポジトリの過去コミット範囲に対し`mise run cover-diff:run -- --base <prev> --mod <app>`で実際に動かし、出力を目視確認する。`mise run verify:changed`がcover-diffの変更を自動でpickupすることも確認する

## File changes

- 新規: `scripts/cover-diff/{cmd/cover-diff/main.go, internal/udiff/, internal/cover/, internal/patch/, internal/report/, testdata/, README.md, go.mod}`
- 新規（scaffold生成）: `.github/workflows/ci_cover_diff.yml`
- 編集: `mise.toml`（`cover-diff:*`タスクブロック追加、`go:test`/`go:lint`の`depends`追加）
- 変更しないもの: `.github/workflows/go_module_ci.yml`（既存パターンをそのまま呼ぶだけ）、`ai-agents/scripts/verify-changed.sh` / `.claude/hooks/test-changed.sh`（`scripts/*/*.go`の汎用パターンで自動追従、編集不要）、`ai-agents/agents/code-review-scanner.md`（別issueへ）

## Risks and mitigations

1. **【最重要】no-stmt判定を誤ると全部ノイズになる。** import/構造体宣言/コメント/閉じ括弧等はcover profileのどのブロックにも属さない。→ プロファイルのブロックに現れない行を`LineStates`のmapに一切キーとして出さない設計にし、呼び出し側で「mapに無い行=no-stmt」として分母から除外する。golden testの最初のケースとしてこれを固定する
2. **package単位カバレッジの死角。** `go test ./...`の既定では各packageは自パッケージのテストにしか計測されない。→ 既定はCIと同じ（`-coverpkg`なし）に揃え、`--coverpkg`をオプトインで用意し、READMEに死角を明記する
3. **テスト再実行コスト。** 既定動作は`go test -coverprofile`を走らせるためgo-verifyやStopフックと二重に回る。→ `--mod`で絞る/`--profile`で既存プロファイルを再利用する逃げ道を用意し、Stopフックには入れない
4. **生成コードの扱い。** `ai-bridge`の`mock_port.go`等。→ `--exclude`を用意し、READMEでmockパスを例示する
5. **diffの取り方の穴。** rename/mode変更のみのhunk、`\ No newline at end of file`、壊れたhunkヘッダ。→ 解釈できないhunkは黙って無視せず`Warnings`に積み、全体をfailさせない（`pipeline-metrics`の`parse_warnings`と同じ思想）
6. **CI検証への影響。** net-newモジュールなので既存7モジュールのCI挙動は変わらない。`go_module_ci.yml`自体は変更しない
7. **AI呼び出し配線を今回含めないことによる「絵に描いた餅」状態の継続。** → Design policyで別issue化を明記し、実装完了後に速やかに起票する運用でカバーする

## Validation

1. `internal/udiff`・`internal/cover`はtable-driven test（golden不要、構造体の`reflect.DeepEqual`比較）で個々の分岐を潰す
2. `cmd/cover-diff`配下でend-to-endのgolden testを固定する（`pipeline-metrics`の`-update`フラグ運用を流用）。`testdata/sample.diff`には以下を1本に混在させる: no-stmt判定用（コメント・空行・構造体フィールド・実ステートメント混在）、rename-only/mode-onlyのファイルブロック、no-newlineマーカー、`+++ /dev/null`の削除ファイル、壊れたhunkヘッダ、`--exclude`適用前後の差分確認、`_test.go`の既定除外確認
3. `--format json`のフィールド名を固定する契約テスト（`pipeline-metrics`の`TestJSONContract`と同様）を`main_test.go`に置く
4. `--threshold`のON/OFFによる終了コードの違いは`execute`の戻り値を直接assertするテストでカバー（goldenにしない）
5. `mise run cover-diff:test` / `cover-diff:lint` / `cover-diff:build`をローカルで通す
6. 実リポジトリの過去コミット範囲に対して実運用コマンドで一度動かし、出力が破綻していないことを目視確認する
7. `mise run verify:changed`がcover-diffの変更を自動でpickupすることを確認する

## Open questions

実装着手前にユーザー確認が必要な論点。

1. **AI呼び出し配線を今回のPRに含めるか。** 推奨: 含めず別issueに切り出す（Design policy参照）
2. **`scaffold --from`の選択。** 推奨: `pipeline-metrics`（`:run`タスク付き）。既定の`config-diff`にする理由があれば再検討
3. **`--profile`も`go test`も無い/失敗する場合の扱い。** `--threshold`指定時にこれを「測定不能」として非ゼロ終了させる（閾値保証をごまかせないようにする）か、警告のみ出して0終了のままにするか
4. **`--base`の差分取得方法。** `git diff --unified=0 <base>...HEAD`（3ドット=merge-base差分、pathspecなし・フィルタはGo側に一任）という設計でよいか。`origin/main`未fetch時のエラーメッセージにヒントを付けるか
5. **`internal/patch`パッケージ新設の採否。** issue本文どおり3パッケージ+`cmd/cover-diff/main.go`に結合ロジックを直接書く方針でも動く
6. **`--exclude`の既定値。** 既定では何も除外しない（`_test.go`だけ常時除外）方針でよいか、`mock`等のパターンを既定値にハードコードするか（`pipeline-metrics`/`go-verify`の流儀では非ハードコード寄りを推奨）
7. **testdataを合成テキストのみで組む方針。** 実際にビルド可能なGoソース+実`go test -coverprofile`出力を使わず、`pipeline-metrics`同様に手で書いた合成diff/profileテキストで固定する、で問題ないか
