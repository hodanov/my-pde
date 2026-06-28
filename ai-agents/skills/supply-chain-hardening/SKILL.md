---
name: supply-chain-hardening
description: GitHub Actions の SHA ピン留め（pinact 必須）と Dependabot・Renovate のクールダウン設定（14日間）をリポジトリに適用し、サプライチェーン攻撃対策を実施する。ユーザーが「サプライチェーン対策」「GHA ピン留め」「Dependabot クールダウン」「pinact」「supply chain hardening」に言及した場合に使用する。
disable-model-invocation: true
argument-hint: "[対象スコープ: all / gha / dependabot]"
metadata:
  version: 1
---

# Supply Chain Hardening

GitHub Actions の SHA ピン留めと Dependabot / Renovate のクールダウン設定を段階的に適用するスキル。

## ワークフロー全体像

```text
Phase 0: pinact インストール確認（必須前提条件）
Phase 1: 現状分析レポート
Phase 2: GitHub Actions SHA ピン留め（pinact 使用）
Phase 3: Dependabot / Renovate クールダウン設定
Phase 4: 検証
```

**スコープ制御**: `$ARGUMENTS` で対象を絞れる。

- `all`（デフォルト）: Phase 2 + 3 の両方
- `gha`: Phase 2 のみ
- `dependabot`: Phase 3 のみ

---

## Phase 0: pinact インストール確認

**pinact は本スキルの必須前提条件である。**

```bash
which pinact && pinact --version
```

- pinact がインストール済み → Phase 1 に進む
- **未インストールの場合 → 作業を中断し、ユーザーにインストールを促す**

インストール方法は公式ドキュメントを案内する:
<https://github.com/suzuki-shunsuke/pinact/blob/main/INSTALL.md>

```text
⚠️ pinact がインストールされていません。
以下のドキュメントを参考にインストールしてください:
https://github.com/suzuki-shunsuke/pinact/blob/main/INSTALL.md

例（Go）:
  go install github.com/suzuki-shunsuke/pinact/cmd/pinact@latest

例（Homebrew）:
  brew install suzuki-shunsuke/pinact/pinact

インストール後に再度このスキルを実行してください。
```

**pinact が確認できるまで以降のフェーズには進まないこと。**

---

## Phase 1: 現状分析レポート

リポジトリの現状を調べ、分析レポートを作成する。

### 1-1. リポジトリの現状を取得

リポジトリルートで調査スクリプトを実行し、現状を自動取得する。

```bash
bash scripts/scan-repo.sh
```

スクリプトは以下の項目を出力する:

- GitHub Actions ワークフロー一覧
- 未ピン留めの外部 Action
- ピン留め済みの Action
- Dependabot / Renovate 設定の存在
- Dependabot の cooldown 設定状況
- Renovate の minimumReleaseAge 設定状況

### 1-2. GitHub Actions 分析

- ワークフローファイル数
- `uses:` の総数
- SHA ピン留め済みの数（`# vX.Y.Z` コメント付き）
- **未ピン留めの外部 Action 一覧**（ファイル名・行番号付き）
- ローカル参照（`./.github/`）の数（ピン留め不要として除外）
- reusable workflow（`*.yml@ref`）の数

### 1-3. Dependabot / Renovate 分析

- 使用している依存管理ツール（Dependabot / Renovate / なし）
- 設定済みのエコシステム一覧
- `cooldown` / `minimumReleaseAge` の設定状況
- 自動マージルールの有無

### 1-4. 分析結果の提示

分析結果をテーブル形式でユーザーに提示する。

```markdown
| カテゴリ   | 項目                   | 状態              |
| ---------- | ---------------------- | ----------------- |
| GHA        | 未ピン留め Action 数   | X 箇所            |
| GHA        | ピン留め済み Action 数 | Y 箇所            |
| Dependabot | cooldown 設定          | 未設定 / 設定済み |
| Renovate   | minimumReleaseAge      | 未設定 / 設定済み |
```

**全て対策済みの場合**: その旨を報告し、Phase 4 の検証のみ実行する。

---

## Phase 2: GitHub Actions SHA ピン留め

詳細手順は [references/gha-pinning-guide.md](references/gha-pinning-guide.md) を参照。

### 2-1. pinact で一括ピン留め

```bash
pinact run .github/workflows/*
```

- `pinact` は全ワークフローの未ピン留め Action を一括で SHA ピン留めする
- `SHA # vX.Y.Z` 形式のコメントも自動付与される

### 2-2. `pinact run --verify` で整合性チェック

```bash
pinact run --verify .github/workflows/*
```

- 既にピン留め済みの Action も含め、SHA とタグの対応が正しいか検証する
- 不整合がある場合はエラーが表示される → `pinact run` で再ピン留めする

### 2-3. 注意事項

- ローカル参照（`./.github/common/` や `./.github/workflows/*.yml`）は **ピン留め対象外**
- reusable workflow（`org/repo/.github/workflows/x.yml@ref`）は SHA ピン留め対象とする
- ピン留め後のバージョンが 2 週間以上前のリリースであることを確認する

---

## Phase 3: Dependabot / Renovate クールダウン設定

詳細手順は [references/dependabot-cooldown-guide.md](references/dependabot-cooldown-guide.md) を参照。

### 3-1. Dependabot の場合

`.github/dependabot.yml` の各エコシステムに以下を追加する:

```yaml
cooldown:
  default-days: 14
```

- 既に `cooldown` が設定済みのエコシステムはスキップする
- `default-days` が 14 未満の場合は 14 に引き上げるかユーザーに確認する

### 3-2. Renovate の場合

`renovate.json`（または `.renovaterc.json`）の `packageRules` に以下を追加する:

```json
{
  "matchManagers": ["github-actions"],
  "minimumReleaseAge": "14 days"
}
```

- 既に `minimumReleaseAge` が設定済みの場合はスキップする
- 全エコシステム共通で適用する場合は `matchManagers` を省略する

### 3-3. どちらも未導入の場合

ユーザーに Dependabot の導入を提案し、`.github/dependabot.yml` のテンプレートを提示する。

---

## Phase 4: 検証

### 4-1. GHA ピン留めの検証

```bash
# 未ピン留めの外部 Action がゼロであること
# SHA（40桁の16進数）が付いていない uses: 行を抽出する
grep -rn 'uses:' .github/workflows/ \
  | grep -v '@[a-f0-9]\{40\}' \
  | grep -v '^\.\/'
# → 0 行であること
```

> **注意**: `@branch` や `@tag` で意図的に参照している Action（自己参照など）がある場合は、
> リポジトリに合わせて追加の `grep -v` で除外すること。除外パターンはユーザーに確認する。

### 4-2. Dependabot / Renovate の検証

```bash
# dependabot.yml の全エコシステムに cooldown が設定されていること
grep -c 'cooldown' .github/dependabot.yml

# renovate の場合
grep 'minimumReleaseAge' renovate.json
```

### 4-3. 結果レポート

最終的な対策状況をテーブル形式で報告する。

```markdown
| カテゴリ            | 対策前 | 対策後 |
| ------------------- | ------ | ------ |
| GHA 未ピン留め      | X 箇所 | 0 箇所 |
| Dependabot cooldown | 未設定 | 14 日  |
```
