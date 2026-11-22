---
name: supply-chain-hardening
description: GitHub Actions の SHA ピン留め（pinact 必須）、Dependabot・Renovate のクールダウン設定（14日間）、自動マージ条件の厳格化、zizmor の導入、リポジトリ設定（既定トークン・SHA 固定の強制・ブランチ保護）をリポジトリに適用し、サプライチェーン攻撃対策を実施する。ユーザーが「サプライチェーン対策」「GHA ピン留め」「Dependabot クールダウン」「pinact」「自動マージの条件」「zizmor」「supply chain hardening」に言及した場合に使用する。
disable-model-invocation: true
argument-hint: "[対象スコープ: all / gha / dependabot / automerge / zizmor / settings]"
metadata:
  version: 2
---

# Supply Chain Hardening

GitHub Actions の SHA ピン留め、Dependabot / Renovate のクールダウン、自動マージの条件、zizmor、リポジトリ設定を段階的に適用するスキル。

## ワークフロー全体像

```text
Phase 0: pinact インストール確認（必須前提条件）
Phase 1: 現状分析レポート
Phase 2: GitHub Actions SHA ピン留め（pinact 使用）
Phase 3: Dependabot / Renovate クールダウン設定
Phase 4: 自動マージの条件
Phase 5: zizmor
Phase 6: リポジトリ設定（GitHub 上の設定変更。1 件ずつ確認を取る）
Phase 7: 検証
```

**スコープ制御**: `$ARGUMENTS` で対象を絞れる。

- `all`（デフォルト）: Phase 2〜6 の全て
- `gha`: Phase 2 のみ
- `dependabot`: Phase 3 のみ
- `automerge`: Phase 4 のみ
- `zizmor`: Phase 5 のみ
- `settings`: Phase 6 のみ

---

## Phase 0: pinact インストール確認

**スコープに Phase 2 を含む（`all` / `gha`）とき、pinact は必須前提条件である。** それ以外のスコープではこの Phase をスキップする。

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

**スコープに Phase 2 を含むときは、pinact が確認できるまで以降のフェーズには進まないこと。**

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
- 自動マージのワークフローと `dependabot/fetch-metadata` の有無
- zizmor の設定と CI での利用
- リポジトリ設定（SHA 固定の強制、既定トークンの権限、Actions による PR 承認、ブランチ保護）

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
| 自動マージ | 条件                   | 無条件 / 条件あり |
| zizmor     | CI での実行            | なし / あり       |
| 設定       | 既定トークン           | write / read      |
| 設定       | SHA 固定の強制         | 無効 / 有効       |
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
- 自動マージを使うリポジトリでは、各グループに `update-types: ["minor", "patch"]` を付けてメジャー更新を別 PR に分ける（Phase 4）

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

## Phase 4: 自動マージの条件

詳細手順は [references/repo-hardening-guide.md](references/repo-hardening-guide.md) を参照。自動マージのワークフローが無いリポジトリではスキップする。

- Dependabot は `dependabot/fetch-metadata` で判定し、semver の patch / minor だけを自動マージする
- GitHub Actions の更新（`package-ecosystem == github_actions`）は自動マージしない
- 自作の bot の PR は、ブランチ名に加えて作成者でも判定する
- `permissions` はトップレベルを `{}` にし、ジョブ単位で与える

---

## Phase 5: zizmor

詳細手順は [references/repo-hardening-guide.md](references/repo-hardening-guide.md) を参照。

- `zizmor .github` を既定の重大度で実行し、指摘を直す。直さない指摘は理由を書いて `.github/zizmor.yml` で抑制する
- 全 PR で走る lint ワークフローに zizmor を足す

---

## Phase 6: リポジトリ設定

詳細手順は [references/repo-hardening-guide.md](references/repo-hardening-guide.md) を参照。**どれも GitHub 上の設定を変えるので、1 件ずつユーザーの確認を取ってから実行する。**

- 既定の `GITHUB_TOKEN` を read にし、Actions による PR の承認を禁止する
- SHA 固定を強制する（Phase 2 で全 `uses:` を固定してから）
- 既定ブランチの保護で、管理者にも適用し、セキュリティ系のチェックを必須にする
- private vulnerability reporting を有効にする

---

## Phase 7: 検証

### 7-1. GHA ピン留めの検証

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

### 7-2. Dependabot / Renovate の検証

```bash
# dependabot.yml の全エコシステムに cooldown が設定されていること
grep -c 'cooldown' .github/dependabot.yml

# renovate の場合
grep 'minimumReleaseAge' renovate.json
```

### 7-3. 自動マージ・zizmor・リポジトリ設定の検証

```bash
# 指摘が 0 件であること（抑制した指摘は suppressed として数えられる）
zizmor .github

# 自動マージの条件とリポジトリ設定を読み直す
bash scripts/scan-repo.sh
```

- 次の Dependabot の PR で、patch / minor だけが自動マージされ、メジャー更新と GitHub Actions の更新が残ることを確かめる

### 7-4. 結果レポート

最終的な対策状況をテーブル形式で報告する。

```markdown
| カテゴリ            | 対策前 | 対策後    |
| ------------------- | ------ | --------- |
| GHA 未ピン留め      | X 箇所 | 0 箇所    |
| Dependabot cooldown | 未設定 | 14 日     |
| 自動マージの条件    | 無条件 | 条件あり  |
| zizmor              | なし   | CI で実行 |
| 既定トークン        | write  | read      |
```
