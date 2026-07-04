# nvim-sync: Neovim 設定をコンテナへ即時同期する

ホスト側の `nvim/config/` を監視し、変更された Lua ファイルを稼働中の `nvim-dev` コンテナへ `docker cp` で自動転送する常駐ウォッチャ。

## 背景

この PDE では Neovim 設定はイメージビルド時に `COPY` で焼き込まれており、bind mount されていない。そのため `nvim/config/**` を 1 行直すたびに、イメージのリビルドか手作業の `docker cp` が必要だった（`scripts/ai-bridge/README.md` の「セットアップ 5」「トラブルシューティング」を参照）。`nvim-sync` はこの手作業をループから消す。

## ビルド

```bash
cd scripts/nvim-sync
go build -o nvim-sync ./cmd/nvim-sync
```

## 使い方

リポジトリのルートから実行する（`NVIM_SYNC_SRC` の既定が `nvim/config` のため）。

```bash
# 起動時に全ファイルを一度コピー（初期同期）
./scripts/nvim-sync/nvim-sync sync

# 変更を監視して逐次同期（常駐）
./scripts/nvim-sync/nvim-sync watch
```

`watch` は再帰的にディレクトリを監視し、`*.lua` の作成・更新を検知して、短時間の連続保存はデバウンスで束ねてから `docker cp` する。コピー先はソースルートからの相対パスを保つ（例: `nvim/config/lua/ai_bridge.lua` → `nvim-dev:/root/.config/nvim/lua/ai_bridge.lua`）。

## 設定

| 環境変数              | デフォルト           | 説明                         |
| --------------------- | -------------------- | ---------------------------- |
| `NVIM_SYNC_CONTAINER` | `nvim-dev`           | 転送先コンテナ名             |
| `NVIM_SYNC_SRC`       | `nvim/config`        | 監視するホスト側ディレクトリ |
| `NVIM_SYNC_DEST`      | `/root/.config/nvim` | コンテナ側の配置先ルート     |

## スコープと今後

最小構成として `watch` / `sync` と構造化ログまでを実装している。変更後の Neovim への `:source` 自動再読込、`--json` ログ、polling フォールバックは後続。`docker cp` の配置先は Dockerfile の `COPY` 先（`/root/.config/nvim/`）と一致させている。

## テスト

```bash
go test ./...
```

`docker` 実行は `syncer.Runner` 注入でスタブ化しており、実 Docker・ネットワークに依存しない。相対パス算出・コピー先パス組み立て・デバウンス束ねをテーブル駆動で検証する。

本モジュール専用の CI として `.github/workflows/ci_nvim_sync.yml` を用意しており、共通の reusable workflow `.github/workflows/go_module_ci.yml` を呼び出して lint（goimports / golangci-lint）と test（`go test` + カバレッジ PR コメント）を実行する。`paths: scripts/nvim-sync/**` または `mise.toml` の変更で起動する。
