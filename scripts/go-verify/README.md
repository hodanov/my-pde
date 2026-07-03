# go-verify

`scripts/` 配下の Go モジュールを自動探索し、各モジュールに対して CI と同一手順の検証を
ローカルで一括実行する shift-left な品質ゲート。`fail` があれば非ゼロ終了する。

## 何を検証するか

`go.mod` を持つディレクトリごとに、CI と同じ順・同じ設定で次を実行する。

1. **goimports** — `goimports -l .` の出力（整形差分のあるファイル一覧）が空でなければ fail。
2. **golangci-lint** — `golangci-lint run ./...`（リポジトリ root の `.golangci.yml` を使用）。
3. **go test** — `go test ./... -count=1`。

## 使い方

```sh
# scripts/ 配下の全 Go モジュールを全チェック
go run ./cmd/go-verify -root ..

# lint だけ / test だけに絞る
go run ./cmd/go-verify -root .. -only lint
go run ./cmd/go-verify -root .. -only test

# パスに "ai-bridge" を含むモジュールだけ
go run ./cmd/go-verify -root .. -mod ai-bridge
```

### フラグ

| フラグ  | 既定 | 説明                                                                    |
| ------- | ---- | ----------------------------------------------------------------------- |
| `-root` | `.`  | Go モジュールを探索するルートディレクトリ                               |
| `-only` | 空   | 実行するチェックを絞る（`lint` = goimports+golangci、`test` = go test） |
| `-mod`  | 空   | パスに指定文字列を含むモジュールだけを対象にする                        |

## 終了コード

- `0` — 全チェック pass
- `1` — いずれかのチェックが fail
- `2` — 使用方法・探索エラー（不正な `-only` 値、モジュールが見つからない等）

## 設計

- 外部依存ゼロ（標準ライブラリのみ）。
- 外部コマンド実行は `runner.Runner` 関数として注入でき、探索・チェック順・集約・終了コードを
  実 `go` / `golangci-lint` やネットワークに依存せず table-driven テストで検証している。
- `vendor` / `testdata` / ドットディレクトリは探索から除外する。

## 前提ツール

ホストの PATH に `goimports` / `golangci-lint` / `go` が必要（未導入のチェックは fail として表示される）。
