# 自動マージ・zizmor・リポジトリ設定ガイド

## 自動マージの条件

更新 PR を無条件に自動マージすると、上流のリリース改ざんがレビューなしで既定ブランチに入る。安全と判定できる PR だけを通し、判定できないものは手動に回す（fail-closed）。

- Dependabot は `dependabot/fetch-metadata` で判定し、semver の patch / minor だけを自動マージする。
- GitHub Actions の更新は自動マージしない。Action の中身は CI の権限で動くので、差分を確認する。
- `update-type` が空の PR（間接依存のセキュリティ更新、digest だけの更新）は、条件に一致しないので手動に残る。
- 自作の bot が作る PR は、ブランチ名だけでなく作成者（App の bot アカウント）でも判定する。ブランチ名は誰でも真似られる。
- `permissions` はトップレベルで `{}` にし、マージするジョブだけに `contents: write` と `pull-requests: write` を与える。

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions: {}

jobs:
  dependabot:
    if: github.event.pull_request.draft == false && github.event.pull_request.user.login == 'dependabot[bot]'
    runs-on: ubuntu-latest
    timeout-minutes: 5
    permissions:
      contents: write
      pull-requests: write
    steps:
      - name: Fetch Dependabot metadata
        id: metadata
        uses: dependabot/fetch-metadata@<SHA> # vX.Y.Z

      - name: Enable auto-merge
        if: |
          steps.metadata.outputs.package-ecosystem != 'github_actions' && (
            steps.metadata.outputs.update-type == 'version-update:semver-patch' ||
            steps.metadata.outputs.update-type == 'version-update:semver-minor'
          )
        env:
          GH_TOKEN: ${{ github.token }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
        run: gh pr merge "$PR_NUMBER" --auto --squash --repo "$GITHUB_REPOSITORY"
```

`package-ecosystem` はブランチ名の一部から取られるので、`github-actions` ではなく `github_actions` になる。グループ PR にメジャー更新が混ざると、`update-type` は最も大きい変更（major）になり自動マージされない。`.github/dependabot.yml` の各グループに `update-types: ["minor", "patch"]` を付け、メジャー更新を別 PR に分ける。

自動マージは必須チェックの通過を待つ。セキュリティ系のチェック（gitleaks、zizmor など）を必須チェックに入れておく。必須チェックにするジョブは `paths` で絞らない（絞ると、対象外の PR で永遠に待つ）。

## zizmor

zizmor は GitHub Actions の静的解析ツールで、テンプレートインジェクション、過剰な権限、`pull_request_target` の誤用、未固定の参照などを見つける。

```bash
zizmor .github
```

- `--min-severity` で絞らず、既定の重大度で回す。low / info にも、`run:` への `${{ }}` の直接展開のように直す価値のある指摘が入る。
- 対象は `.github` 全体にする。`dependabot.yml` と composite action（`.github/actions/*/action.yml`）も検査される。
- `run:` に `${{ steps.*.outputs.* }}` や `${{ github.event.* }}` を直接書いている指摘は、値を `env:` に移して `"$VAR"` で参照する形に直す。
- 直さない指摘は、理由を書いて `.github/zizmor.yml` で抑制する。

  ```yaml
  rules:
    <audit-name>:
      # <抑制する理由>
      disable: true
  ```

- CI では lint のワークフローに zizmor のステップを足し、全 PR で走らせる。オンラインの検査（Action の既知の脆弱性など）には `GH_TOKEN` を渡す。

## リポジトリ設定

どれも GitHub 上の設定を変える操作なので、**1 件ずつユーザーの確認を取ってから実行する**。`<owner>/<repo>` は `gh repo view --json nameWithOwner --jq .nameWithOwner` で取る。

| 設定                                 | 効果                                                           | コマンド                                                                                                                                         |
| ------------------------------------ | -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| 既定の `GITHUB_TOKEN` を read にする | `permissions:` を書き忘れたワークフローが write 権限を持たない | `gh api -X PUT repos/<owner>/<repo>/actions/permissions/workflow -f default_workflow_permissions=read -F can_approve_pull_request_reviews=false` |
| Actions による PR の承認を禁止する   | 乗っ取られたワークフローが、自分の PR を承認してマージできない | 上と同じコマンド（`can_approve_pull_request_reviews=false`）                                                                                     |
| SHA 固定を強制する                   | タグやブランチで参照する Action をワークフローが使えなくなる   | `gh api -X PUT repos/<owner>/<repo>/actions/permissions -F enabled=true -f allowed_actions=all -F sha_pinning_required=true`                     |
| 管理者にもブランチ保護を適用する     | 管理者のトークンで既定ブランチへ直接 push できない             | `gh api -X POST repos/<owner>/<repo>/branches/<branch>/protection/enforce_admins`                                                                |
| 必須チェックを足す                   | セキュリティ系のチェックが落ちた PR はマージされない           | `gh api -X POST repos/<owner>/<repo>/branches/<branch>/protection/required_status_checks/contexts -f 'contexts[]=<check-name>'`                  |
| private vulnerability reporting      | 脆弱性を非公開で報告してもらえる                               | `gh api -X PUT repos/<owner>/<repo>/private-vulnerability-reporting`                                                                             |

- SHA 固定を強制する前に、全ての `uses:` が SHA で固定されていることを確かめる（Phase 2）。固定していない Action があると、そのワークフローが失敗する。
- `allowed_actions` を `selected` にしているリポジトリでは、既存の値を `gh api repos/<owner>/<repo>/actions/permissions` で読み、同じ値を渡す。
- 既定トークンを read にする前に、`permissions:` を書いていないワークフローが無いか確かめる。書いていないワークフローで書き込みが要るものは、先に `permissions:` を足す。
- 必須チェックの名前は、ワークフロー名ではなくジョブの名前（PR のチェック一覧に出る名前）を使う。
- secret scanning の汎用パターンと validity checks は、有料の GitHub Secret Protection が要る。public リポジトリでも有効にできないことがある。
