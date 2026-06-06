# 変換例

雑なメモ・会話の断片から、`template.md` に沿った実装プランへ整形する例。
原文に無い設計判断は足さず、表現と構造だけを整える点に注意する。

## 例1: 会話メモ → 実装プラン

### Before（入力）

```text
ログ出力スキル、保存先がハードコードでdocs/logになってて困る。
環境変数で変えたい。あとCLIオプションでも上書きできると嬉しい。
優先順位は CLIオプション > 環境変数 > デフォルト(docs/log) でいきたい。
環境変数名は LOG_OUTPUT_DIR にする。
リスクとしては既存のdocs/logに出してる人が壊れるかも。デフォルト維持なら大丈夫なはず。
テストは、env未設定時にdocs/logになること、env設定時にそっち、オプション指定時に最優先、を確認。
```

### After（出力）

```markdown
# Plan: ログ出力先の設定可能化

ログ保存先を環境変数と CLI オプションで上書き可能にする。現状の docs/log ハードコードを解消し、デフォルト挙動は維持する。

## Background

- 保存先が docs/log にハードコードされており変更できない
- 環境変数・CLI オプションでの上書きが必要

## Design policy

- 優先順位は CLI オプション > 環境変数 > デフォルト（docs/log）
- 環境変数名は `LOG_OUTPUT_DIR`
- デフォルト値を維持し、既存利用者の挙動を変えない

## Implementation steps

1. 保存先の解決ロジックを「CLI オプション → 環境変数 → デフォルト」の順で実装する
2. `LOG_OUTPUT_DIR` の読み取りを追加する
3. CLI オプションを追加する

## Risks and mitigations

| Risk                                 | Mitigation                         |
| ------------------------------------ | ---------------------------------- |
| 既存の docs/log 利用者の挙動が変わる | デフォルト値を docs/log に維持する |

## Validation

- [ ] 環境変数・オプション未設定時に docs/log に出力される
- [ ] `LOG_OUTPUT_DIR` 設定時にその値が使われる
- [ ] CLI オプション指定時に最優先で使われる
```

### このとき行ったこと

- 断片的な発言を `Background` / `Design policy` / `Implementation steps` / `Risks` / `Validation` に再配置した
- 「env未設定時にdocs/log…」という口語のテスト観点をチェックリスト化した
- `LOG_OUTPUT_DIR` という識別子は原文表記のまま保持した
- 入力に無い `Current structure` と `Open questions` は省略した（無理に埋めない）
