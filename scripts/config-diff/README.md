# config-diff

リポジトリの `ai-agents/` 内「設定ソース」と、デプロイ先（`~/.claude` / `~/.cursor` /
`~/.codex` / `~/.copilot`）の実体を **read-only** で比較し、`ok` / `drift`（内容が食い違う）/
`missing`（未デプロイ）を集約表示する差分ツール。**一切コピーしない。** `drift` / `missing` が
あれば非ゼロ終了する。

`ai-agents/scripts/copy-entries.sh` と同じ `<mode> <src> <dest>` 契約・同じ列挙規則を取るので、
「copy が触る対象」と「diff が見る対象」が必ず揃う。

## 使い方

```sh
config-diff <mode> <src> <dest>    # mode: skills | agents | settings
```

例（`ai-agents/Makefile` の `*_SRC` / `*_DEST` を流用する想定）:

```sh
config-diff settings ai-agents/settings/claude ~/.claude
config-diff skills   ai-agents/skills          ~/.claude/skills
config-diff agents   ai-agents/agents          ~/.claude/agents
```

## 列挙規則（copy-entries.sh と一致）

| mode       | 列挙対象                       | label            | 比較単位             |
| ---------- | ------------------------------ | ---------------- | -------------------- |
| `skills`   | `src` 直下のディレクトリ       | basename         | ディレクトリ再帰比較 |
| `agents`   | `src` 直下の `*.md` ファイル   | basename         | ファイル（sha256）   |
| `settings` | `src` 配下の全ファイル（再帰） | `src` からの相対 | ファイル（sha256）   |

## 判定

- `missing` — dest に対応エントリが無い
- `drift` — 存在するが内容が違う（`skills` はディレクトリ内の `drift:` / `missing:` / `extra:` を列挙）
- `ok` — 一致

`extra:`（dest 側のみに存在するファイル）は `skills` のディレクトリ比較でのみ検出する。`cp -R` は
`rm -rf` 後に上書きするため、この余剰ファイルはデプロイ時に消える対象を表す。

## 終了コード

- `0` — 全エントリ ok
- `1` — drift / missing が 1 つ以上
- `2` — 使用方法・ソース不明・不正な mode 等

## 比較の意味論

- 内容比較は **sha256**（バイト一致）。バイト列そのものを比較するため、改行コードの違い
  （LF / CRLF）も drift になる。一方、sha256 に含まれないファイルモード（実行ビット）や
  `cp -p` が保存する mtime は drift とみなさない。
- シンボリックリンク（`codex` の `AGENTS.md` は Makefile で `ln -sf`）は copy 対象外なので
  check でも対象外。

## 設計

- 外部依存ゼロ（標準ライブラリのみ）。マッピングは持たず `(mode, src, dest)` を受けるだけで、
  単一情報源は `ai-agents/Makefile` に保つ。
- 列挙・分類・再帰比較・集約・終了コードを `t.TempDir()` に組み立てた src/dest で table-driven
  テスト（実ホーム・ネットワーク非依存）。
