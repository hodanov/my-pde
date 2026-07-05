# scaffold

`scripts/` 配下に**新しい Go モジュールを 1 コマンドで生成する**ジェネレータ。Go 骨格
（`go.mod` / `cmd/<name>/main.go` + テスト / `README.md`）に加え、対になる CI ワークフロー
（`.github/workflows/ci_<name>.yml`）を生成し、`mise.toml` へ貼るタスクブロックを標準出力に出す。

新モジュールを足すたびに CI ワークフローと mise タスクを手作業でコピペ・書き換えする運用は、
ペアのワークフローを作り忘れるとそのモジュールが **CI のパススコープから丸ごと外れる**（検証
されない）取りこぼしを生む。scaffold は **CI 配線とタスク雛形を生成時に構造的に用意**して
このクラスのミスを消す。

## 使い方

```sh
# リポジトリルートで実行する
scaffold new <name> [--from <module>] [--root <dir>]

# 例
scaffold new log-tail
```

- `<name>` はモジュール名（lowercase kebab-case、例 `log-tail`）。
- `--from` は CI / mise のテンプレート元モジュール（既定 `config-diff`）。
- `--root` はテンプレートを読み・生成物を書くリポジトリルート（既定 `.`）。

生成後の手順:

1. 出力された mise タスクブロックを `mise.toml` 末尾へ貼る。
2. `go:test` / `go:lint` の `depends` に `<name>:test` / `<name>:lint` を追加する。
3. `cmd/<name>/main.go` の `execute` に実装を書き、`mise run <name>:test` / `<name>:lint` で検証する。

## 設計

- **外部依存ゼロ**（標準ライブラリのみ）。生成は一発実行。
- **テンプレートは生きた既存ファイルから**取る。CI ワークフローと mise タスクは
  テンプレート元モジュール（既定 `config-diff`）の実ファイルを読み、モジュール名トークンを
  置換して生成するため、pinned action SHA や `paths:` などの現行規約が自動的に引き継がれ、
  テンプレートがリポジトリの実体から drift しない。Go 骨格だけは pinned SHA を持たない
  最小テンプレート。
- **安全性**：生成は付加のみ。生成先が既に存在する場合は**上書きせず非ゼロ終了**し、
  既存ファイルは一切 mutate しない。`mise.toml` も in-place 編集はせず、貼るべきブロックを
  stdout に出すだけ。
- 生成ロジック（`internal/gen`）は reader / exists コールバックを受ける純粋関数で、
  `t.TempDir()` 非依存の table-driven テストで網羅する。

## 終了コード

- `0` — 生成成功
- `1` — 生成エラー（テンプレート読み込み失敗・生成先の衝突など）
- `2` — 使用方法エラー
