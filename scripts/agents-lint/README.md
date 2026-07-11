# agents-lint

`ai-agents/` 配下のスキル / サブエージェント定義を **read-only** で静的検証する Go リンタ。
frontmatter スキーマ・命名規約・`name` とディレクトリ/ファイル名の一致・name 重複・
スキルが起動するサブエージェント参照の実在を機械検証し、違反があれば非ゼロ終了する。

`ai-agents/` を single source of truth として 4 CLI（claude / cursor / copilot / codex）へ
push デプロイする経路の「**壊れた定義を配る前の足切り**」。既存の lint（prettier /
markdownlint）が体裁しか見ないのに対し、こちらは frontmatter のスキーマ・規約・参照整合を見る。

## 使い方

```sh
agents-lint [--root ai-agents] [--strict]
```

- `--root`: `skills/` と `agents/` を持つルート（既定 `ai-agents`）
- `--strict`: warn も失格扱いにする

Exit code: `0` = クリーン / `1` = lint 違反（error あり、または `--strict` で warn あり） / `2` = usage・IO エラー。

## 検査対象と選別

`ai-agents/scripts/copy-entries.sh` の modes に一致させ、「配る対象＝検証する対象」を揃える。

- **skills** = `<root>/skills/<name>/SKILL.md`（直下ディレクトリ）
- **agents** = `<root>/agents/<name>.md`（直下 `*.md`）

## ルール

| 対象   | ルール（slug）                                                               | Sev   |
| ------ | ---------------------------------------------------------------------------- | ----- |
| skills | `frontmatter-present` / `frontmatter-closed`（`---`…`---` が閉じている）     | error |
| skills | `name-required` / `name-format` / `name-reserved`（kebab・64字・予約語なし） | error |
| skills | `name-matches-dir`（name == ディレクトリ名）                                 | error |
| skills | `description-required`                                                       | error |
| skills | `name-unique`（skills 間の name 重複）                                       | error |
| skills | `skill-md-present`（ディレクトリに SKILL.md がある）                         | error |
| skills | `ref-agent-exists`（`subagent_type` 参照先が `agents/<name>.md` に実在）     | error |
| skills | `frontmatter-unknown-key`（既知キー外）                                      | warn  |
| agents | `frontmatter-present` / `name-matches-file` / `description-required`         | error |
| agents | `name-required` / `name-format` / `name-reserved`（skills と同じ命名規約）   | error |
| agents | `tools-present` / `model-present`                                            | warn  |
| agents | `frontmatter-unknown-key`                                                    | warn  |

### 仕様追従の設計

Agent-Skills 仕様に直結するのは「どの frontmatter キーが既知か」だけ。既知キーは
`internal/lint/lint.go` の `skillKnownKeys` / `agentKnownKeys` テーブルに集約してあり、
仕様変更時はこのテーブルだけ更新すればよい。未知キーは **error ではなく warn（fail-open）**
なので、仕様にフィールドが増えてもデプロイゲートを即座に壊さない。

### 参照抽出

散文中の言及は誤検出源なので拾わず、明示的な `subagent_type` シグナルにアンカーする
（backtick 形式・`subagent_type: name` 一体形式・Markdown テーブル列の 3 形態）。
backtick 形式で参照として扱うのは `subagent_type` スパンの**直後のスパンのみ**で、
同一行の他のスパンは散文として無視する。テーブルのヘッダ・セルは backtick 付きでも解決する。

## 開発

```sh
mise run agents-lint:test    # go test ./...
mise run agents-lint:lint    # golangci-lint + goimports
mise run agents-lint:build   # バイナリ生成
```
