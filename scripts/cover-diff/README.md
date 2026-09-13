# cover-diff

差分が**追加・変更した行のうち、テストが 1 度も実行していない行**を行番号つきで出す **read-only** な CLI。
いわゆる patch coverage / diff coverage。CI（`.github/workflows/ci-go-module.yml`）が出すのは
モジュール全体の総 % と全関数ダンプで、差分とは紐づいていない。総 % は数十行足しても小数第一位しか
動かないため、「この PR が足した行にテストが当たっているか」はそこからは読み取れない。

このリポジトリは Routine が PR の大半を自動生成し、人間側に残る工程は実質レビューだけになっている。
「誰も 1 度も実行していない新規行」の一覧は、差分のどこに目を寄せるかを決めるための安いシグナルになる。

## 設計の前提

- **解析コアは git も go も呼ばない。** `--diff` と `--profile` を渡せば、ネットワーク・git・`go test` から
  完全に切り離して動く。`testdata/` の固定入力に対するゴールデンテストで出力をバイト単位に固定できる。
- **一切書き込まない。** 生成したカバレッジプロファイルは一時ディレクトリに置いて破棄する。
  リポジトリにも Issue / PR にも触らない。
- **ステートメントが無い行は分母に入れない。** import・型宣言・コメント・空行・閉じ括弧は
  プロファイルのどのブロックにも属さない。これらを「未カバー」に倒すと出力が使い物にならないので、
  ブロックが張っていない行は covered / uncovered のどちらにも数えない。
- **測定できなかったことを「通った」と表示しない。** プロファイルを取れなかったモジュールは
  `unmeasured` として明示し、`--threshold` 指定時は非ゼロ終了する。

## 使い方

```sh
# 既定: origin/main との merge-base 差分を取り、変更のあったモジュールでだけテストを回す
mise run cover-diff:run

# 変更モジュールを絞る / 閾値を課す
mise run cover-diff:run -- --base HEAD~5 --mod agent-stats --threshold 80

# 生成コードを分母から外す
mise run cover-diff:run -- --exclude '/mock/|/cmd/'

# git も go も起動しない解析だけのモード
go run ./cmd/cover-diff --diff patch.diff --profile agent-stats=coverage.out
```

| フラグ        | 既定                            | 意味                                                                      |
| ------------- | ------------------------------- | ------------------------------------------------------------------------- |
| `--root`      | `git rev-parse --show-toplevel` | リポジトリルート。`--diff` と `--profile` が揃っていれば使わない          |
| `--base`      | `origin/main`                   | `git diff --unified=0 <base>...HEAD` の base（3 ドット＝merge-base 差分） |
| `--mod`       | 空                              | 名前に含む文字列でモジュールを絞る                                        |
| `--diff`      | 空                              | unified diff をファイル / `-`（stdin）から注入する。git を呼ばない        |
| `--profile`   | 空                              | 既存プロファイルを使う（`<module>=<path>`、繰り返し可）。go を呼ばない    |
| `--exclude`   | 空                              | パスがこの正規表現に一致する変更ファイルを除外する                        |
| `--coverpkg`  | false                           | `-coverpkg=./...` を付ける（下記「制約」参照）                            |
| `--threshold` | 未指定                          | patch coverage がこの % を下回ったら失敗させる。未指定なら常に成功        |
| `--format`    | `text`                          | `text` / `json`                                                           |

## 正規化ルール

- **変更行**は unified diff の**新しい側**の行番号。`--unified=0` を前提にするが、文脈行つきの
  diff を `--diff` で渡しても同じ結果になる。削除ファイル（`+++ /dev/null`）、rename のみ・
  mode 変更のみのブロックは行を持たないので何も寄与しない。
- **対象は `scripts/<module>/**.go` のみ。** `_test.go` は常時除外する（テスト自身の
  カバレッジを測っても変更の裏付けにならない）。それ以外の除外は `--exclude` で明示する。
- **モジュール判定**は `scripts/` 直下の要素名。`verify-changed.sh` / `test-changed.sh` と同じ規約。
- **座標系の変換**: プロファイルのパスは import path（`agent-stats/internal/report/report.go`）、
  diff のパスはリポジトリ相対（`scripts/agent-stats/internal/report/report.go`）。前者の第 1 セグメントを
  モジュール名として `scripts/<module>/` に読み替える。一致しないパスは警告に出して読み飛ばす。
- **行の 3 値分類**: プロファイルのブロック `path:sL.sC,eL.eC numStmt count` を行へ展開し、
  `count > 0` を covered とする。同じ行を複数ブロックが張る場合は OR。ブロックの終端位置は
  排他なので、`eL.1` で終わるブロックは `eL` を含めない（閉じ括弧が分母に入らない）。
  どのブロックにも属さない行は分母から外す。
- **解釈できない hunk・プロファイル行は黙って捨てない。** hunk ヘッダの破損は警告に積んで残りを続行し、
  プロファイルの 1 行目 `mode:` が未知の形式ならそのモジュールを `unmeasured` にする。

## 出力

- `text` — モジュールごとに `changed / covered / uncovered` と %、その下に未カバー行のレンジ
  （`48-50,55`）、末尾に全体の patch coverage と閾値判定。測定できなかったモジュールと警告は最後に出す。
- `json` — レポート全体。トップレベルは `modules` / `total` / `unmeasured_modules` / `warnings` /
  `threshold`。フィールド名は将来の consumer との契約で、`cmd/cover-diff/main_test.go` の
  `TestJSONContract` が固定している。

## 終了コード

- `0` — 正常終了（閾値未指定、または閾値を満たした）
- `1` — patch coverage が `--threshold` を下回った
- `2` — 使用方法エラー、または `--threshold` 指定時に測定できないモジュールがあった

## 構成

```text
scripts/cover-diff/
  cmd/cover-diff/main.go  # フラグ解釈 → 収集 → 解析 → 出力、終了コード
  internal/udiff/         # unified diff -> ファイル別の追加行番号（純粋パーサ）
  internal/cover/         # cover profile -> ファイル別のステートメント行（純粋パーサ）
  internal/patch/         # 座標系変換・グルーピング・突き合わせと、唯一の exec 境界
  internal/report/        # text / json レンダラ
  testdata/               # 合成の diff / profile とゴールデン
```

標準ライブラリのみに依存する。外部コマンドの実行は `patch.Runner` として注入され、
`cmd/cover-diff/main.go` の `defaultRunner` だけが `exec.Command` を触る。

## 開発

```sh
mise run cover-diff:test
mise run cover-diff:lint

# 出力を変えたときはゴールデンを更新する（差分をレビューすること）
cd scripts/cover-diff && go test ./... -update
```

`testdata/sample.diff` は実データのパターン（ステートメントの無い変更行・rename のみ・
mode 変更のみ・削除ファイル・改行なしマーカー・壊れた hunk ヘッダ・`_test.go`・生成コード・
複数モジュール・`scripts/` 外のファイル）を 1 本に混ぜた合成 diff。
`broken.profile` は「プロファイルを読めなかったモジュール」の経路を実際に通す。

## 制約

- **package 単位カバレッジの死角。** `go test ./...` の既定では、各 package は自分自身のテストにしか
  計測されない。package A のコードが package B のテスト経由でしか実行されない場合、実際には動いていても
  `uncovered` に出る。`--coverpkg` で回避できるが、今度は全 package が分母に入って数字の意味が変わる。
  既定は CI（`ci-go-module.yml`）と同じ `-coverpkg` なしに揃えてある。
- **`<base>...HEAD` はコミット済みの差分だけを見る。** 作業ツリーの未コミット変更は対象外。
  それを測りたいときは `git diff --unified=0 | cover-diff --diff -` で渡す。
- **「module 名 == ディレクトリ名」に依存している。** 座標系の変換がこの規約を前提にしている。
  `scripts/<dir>/go.mod` の module 名を別名にすると、そのモジュールは警告つきで測定不能になる。
- **既定動作は `go test` を回す。** `go-verify` や Stop フックと二重に走るので、`--mod` で絞るか
  `--profile` で既存のプロファイルを使い回す。Stop フックには入れていない。
- 閾値による失敗は `--threshold` を明示したときだけ起きる。CI への組み込みは別スコープ。
