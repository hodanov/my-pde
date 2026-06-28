# AI Bridge: Neovim ↔ AI CLI 連携ガイド

Dockerコンテナ内のNeovimで選択したコードを、ホスト側のAI CLI（Claude Code / Cursor等）にコンテキスト付きで渡す連携機構。

## 前提条件

- ホスト側に以下がインストール済みであること
  - [Go](https://go.dev/) 1.22 以上（ビルド時のみ）
  - AI CLI（例: `claude`、`cursor`）
- WezTermを使用する場合: `wezterm cli` コマンドが使えること
- tmuxを使用する場合: アクティブなtmuxセッションがあること

## セットアップ

### 1. ブリッジディレクトリの作成

```bash
mkdir -p ~/.ai-bridge
```

### 2. Dockerコンテナの再起動

`docker-compose.yml` に追加された `~/.ai-bridge` ボリュームマウントを反映する。

```bash
docker compose -f environment/docker/docker-compose.yml down
docker compose -f environment/docker/docker-compose.yml up -d
```

### 3. バイナリのビルド

```bash
cd scripts/ai-bridge
go build -o ai-bridge ./cmd/ai-bridge
```

### 4. デーモンのセットアップ

**launchd による自動起動（推奨）:**

```bash
./scripts/ai-bridge/ai-bridge install-launchd
```

plist を `~/Library/LaunchAgents/` に生成し、`launchctl load` まで行う。

**手動起動:**

```bash
./scripts/ai-bridge/ai-bridge daemon
```

### 5. Neovim設定の反映

コンテナにLua設定を転送する（イメージリビルドまでの暫定対応）。

```bash
docker cp nvim/config/lua/ai_bridge.lua nvim-dev:/root/.config/nvim/lua/ai_bridge.lua
```

コンテナ内のNeovimで:

```vim
:source ~/.config/nvim/init.lua
```

## 使い方

1. コンテナ内のNeovimでコードをビジュアル選択する（`v` または `V`）
2. `<Space>ai` を押す
3. フローティングウィンドウが開き、コンテキスト付きのプロンプトが表示される
4. プロンプトを確認・編集する
5. `<CR>`（Enter）で送信 → ホスト側のターミナルタブでAI CLIが起動する

### 履歴と再送（history / replay）

デーモンは正常に受理したリクエスト（プロンプト・cwd・タイムスタンプ）を `~/.ai-bridge/history.jsonl` に追記する。Neovim を開かずホスト側だけで過去のプロンプトを確認・再送できる。

```bash
# 直近の履歴を一覧（既定 20 件、-n で件数指定）
./scripts/ai-bridge/ai-bridge history
./scripts/ai-bridge/ai-bridge history -n 5

# 直前のプロンプトを再送（request.json を書き出し、稼働中のデーモンが起動する）
./scripts/ai-bridge/ai-bridge replay --last
```

プロンプト全文が平文で `history.jsonl` に残る点に注意する（`~/.ai-bridge/` 配下に閉じる）。

### フローティングウィンドウのキーマップ

| キー    | 動作                                               |
| ------- | -------------------------------------------------- |
| `<CR>`  | プロンプトを送信してAI CLIを起動                   |
| `<Esc>` | キャンセルしてウィンドウを閉じる                   |
| `<C-[>` | キャンセルしてウィンドウを閉じる（`<Esc>` と同等） |

## 設定

デーモンの動作は環境変数で切り替えられる。

| 環境変数             | デフォルト     | 説明                       |
| -------------------- | -------------- | -------------------------- |
| `AI_BRIDGE_CLI`      | `claude`       | 使用するAI CLIコマンド     |
| `AI_BRIDGE_LAUNCHER` | `wezterm`      | ターミナルランチャー       |
| `AI_BRIDGE_DIR`      | `~/.ai-bridge` | ブリッジディレクトリのパス |

launchd を使用する場合は `~/Library/LaunchAgents/com.ai-bridge.daemon.plist` の `EnvironmentVariables` を編集して `launchctl unload/load` で再起動する。

### AI CLIの切り替え例

**Cursor CLIに切り替える（手動起動）:**

```bash
AI_BRIDGE_CLI=cursor ./scripts/ai-bridge/ai-bridge daemon
```

**新しいAI CLIを追加する:**

`AI_BRIDGE_CLI` に指定するコマンドが `<cmd> "<prompt>"` の形式で動けば、追加設定なしで使用できる。

### ランチャーの切り替え例

**tmuxに切り替える（手動起動）:**

```bash
AI_BRIDGE_LAUNCHER=tmux ./scripts/ai-bridge/ai-bridge daemon
```

**新しいランチャーを追加する:**

`scripts/ai-bridge/internal/launcher/` に Go ファイルを追加し、`Launcher` インターフェースを実装する。

```go
// Launcher opens a new terminal tab and runs a script.
type Launcher interface {
    Launch(cwd, scriptPath string) error
}
```

実装後、`launcher.New()` の switch 文に新しいランチャーを追加する。

## デーモンの管理

| 操作             | コマンド                                                             |
| ---------------- | -------------------------------------------------------------------- |
| 自動起動を有効化 | `launchctl load ~/Library/LaunchAgents/com.ai-bridge.daemon.plist`   |
| 自動起動を無効化 | `launchctl unload ~/Library/LaunchAgents/com.ai-bridge.daemon.plist` |
| 手動起動         | `./scripts/ai-bridge/ai-bridge daemon`                               |
| ログ確認         | `tail -f /tmp/ai-bridge-daemon.log`                                  |

## アーキテクチャ

```text
┌─────────────────────────────┐     ┌──────────────────────────────┐
│  Docker Container           │     │  macOS Host                  │
│                             │     │                              │
│  Neovim                     │     │  ai-bridge-daemon            │
│   ├─ Visual select code     │     │   ├─ ポーリングで監視        │
│   ├─ <Space>ai 押下         │     │   ├─ request.json を読む     │
│   ├─ フローティングウィンドウ│     │   └─ launcher で新規タブ起動  │
│   ├─ プロンプト編集         │     │       └─ <AI_CLI> "prompt"   │
│   └─ <CR> で書き出し ───────┼──→──┤                              │
│                             │     │                              │
│  /.ai-bridge/               │ vol │  ~/.ai-bridge/               │
│    └─ request.json          │ ════│    └─ request.json           │
└─────────────────────────────┘     └──────────────────────────────┘
```

通信はホストとコンテナで共有する `~/.ai-bridge/` ディレクトリ上のJSONファイルを介して行われる。
`${HOME}/workspace:/${HOME}/workspace` のボリュームマウントにより、ファイルパスはコンテナ・ホスト間で一致するため、AI CLIがホスト側からそのままファイルを参照できる。

## トラブルシューティング

**フローティングウィンドウが開かない:**

コンテナに最新の `ai_bridge.lua` が反映されていない可能性がある。

```bash
docker cp nvim/config/lua/ai_bridge.lua nvim-dev:/root/.config/nvim/lua/ai_bridge.lua
```

**デーモンがリクエストを検知しない:**

ブリッジディレクトリの共有ができているか確認する。

```bash
# コンテナ内
ls -la /.ai-bridge/

# ホスト側
ls -la ~/.ai-bridge/
```

**AI CLIが起動しない:**

デーモンのログを確認する。

```bash
tail -f /tmp/ai-bridge-daemon.log
```

ランチャーのコマンド（`wezterm` や `tmux`）がPATHに存在するか確認する。

```bash
which wezterm
which tmux
```
