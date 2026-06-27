---
name: credential-leak-prevention
description: gitleaks + pre-commit を使ってローカルコミット時のクレデンシャル混入を機械的にブロックする。「クレデンシャル漏洩防止」「シークレット混入対策」「gitleaks」「pre-commit シークレット」「credential leak prevention」に言及した場合に使用する。
disable-model-invocation: true
metadata:
  version: 1
---

# Credential Leak Prevention

gitleaks + pre-commit を導入し、ローカルコミット時にシークレット（API キー・トークン等）が混入するのを機械的にブロックするスキル。

## ワークフロー全体像

```text
Phase 0: 前提ツール確認
Phase 1: 現状スキャン
Phase 2: 自動セットアップ
Phase 3: 動作検証
```

---

## Phase 0: 前提ツール確認

**pre-commit と gitleaks は本スキルの必須前提条件である。**

```bash
which pre-commit && pre-commit --version
which gitleaks && gitleaks version
```

どちらかが未インストールの場合、**作業を中断してユーザーにインストールを促す**:

```text
⚠️ 以下のツールが未インストールです。

インストール方法（Homebrew）:
  brew install pre-commit gitleaks

インストール後に再度このスキルを実行してください。
```

両方確認できたら Phase 1 に進む。

---

## Phase 1: 現状スキャン

調査スクリプト [scripts/scan.sh](scripts/scan.sh) を実行し、現状を取得する。

スクリプトは以下の状態を出力する:

- `pre-commit` / `gitleaks` のインストール状況とバージョン
- `.pre-commit-config.yaml` の有無と gitleaks hook の設定状況
- `.gitleaks.toml` の有無と `useDefault = true` の設定状況
- `.git/hooks/pre-commit` の有無（git hook が有効か）

スキャン結果をテーブル形式でユーザーに提示する:

| 項目                    | 状態              |
| ----------------------- | ----------------- |
| pre-commit インストール | ✓ / ✗             |
| gitleaks インストール   | ✓ / ✗             |
| .pre-commit-config.yaml | 存在 / なし       |
| gitleaks hook 設定      | 済 / 未設定       |
| .gitleaks.toml          | 存在 / なし       |
| useDefault = true       | 設定済み / 未設定 |
| git hook 有効化         | 済 / 未実施       |

**全て対策済みの場合**: その旨を報告し、Phase 3 の検証のみ実行する。

---

## Phase 2: 自動セットアップ

ユーザーに確認を取ってからセットアップスクリプト [scripts/setup.sh](scripts/setup.sh) を実行する。

スクリプトは以下を冪等に実行する:

1. `.pre-commit-config.yaml` に gitleaks hook を追加
   - ファイルなし → 新規作成
   - ファイルあり・gitleaks なし → gitleaks ブロックを末尾に追記
   - gitleaks 設定済み → スキップ
2. `.gitleaks.toml` を作成（既に存在する場合はスキップ）
3. `pre-commit install` を実行

### 注意事項

- `.gitleaks.toml` が既に存在する場合はスキップするため、既存の設定を壊さない
- `.pre-commit-config.yaml` が複雑な構造の場合、追記した gitleaks ブロックの位置が適切か目視確認する
- 詳細は [references/gitleaks-guide.md](references/gitleaks-guide.md) を参照

---

## Phase 3: 動作検証

動作検証用のスクリプト [scripts/validate.sh](scripts/validate.sh) を実行する。

スクリプトはダミーの GitHub PAT を含むファイルを一時作成し、`pre-commit run gitleaks` でブロックされることを自動確認する。ファイルは検証後に自動削除される。

**期待結果**: `Failed` が表示され、exit code 1 で終了する。

検証結果をテーブル形式で報告する:

| 検証項目                                       | 結果  |
| ---------------------------------------------- | ----- |
| gitleaks hook がシークレットを検出してブロック | ✓ / ✗ |
| .gitleaks.toml の useDefault = true            | ✓ / ✗ |
| git hook が有効化されている                    | ✓ / ✗ |

全て ✓ なら導入完了。

---

## Phase 4: バージョンの継続的更新（オプション）

`.pre-commit-config.yaml` の `rev` は定期的に更新しないと、フックが陳腐化しセキュリティ上の穴が生じる。

ユーザーに下記を提案して終了する（提案だけ。修正はユーザーに任せる）。

```text
`.pre-commit-config.yaml` の `rev` は定期的に更新しないと、フックが陳腐化しセキュリティ上の穴が生じます。

`pre-commit autoupdate` で手動更新するか、dependabotなどで自動更新してください。

Dependabot による自動更新の設定例↓

---
# `.github/dependabot.yml` に `pre-commit` エコシステムを追加する:
version: 2
updates:
  - package-ecosystem: "pre-commit"
    directory: "/"
    schedule:
      interval: "weekly"
      day: "monday"
    cooldown:
      semver-patch-days: 3 # パッチは 3 日後
      semver-minor-days: 5 # マイナーは 5 日後
      semver-major-days: 7 # メジャーは 7 日後
---

`cooldown` により、リリース直後の不安定なバージョンへの即時更新を防ぐ。
```
