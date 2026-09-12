# ADR-0002: Use a GitHub App for workflow bot authentication

- Status: Accepted (2026-09-12)

## Context

`.github/workflows/automation-tools-bump.yml` が毎週作る bump PR で CI が一度も完了していない。#779 では 14 本すべての workflow run が `action_required`（承認待ち）で止まり、`[CI] Docker Build` も `[Check] Pins Sync` も `[Automation] Deps Auto Merge` も実行されていない。

```text
$ gh run list --branch chore/bump-tool-versions
[CI] Docker Build   event=pull_request  conclusion=action_required  actor=github-actions[bot]
[Check] Pins Sync   event=pull_request  conclusion=action_required  actor=github-actions[bot]
```

原因は `peter-evans/create-pull-request` が既定の `GITHUB_TOKEN` で PR を作っていること。`GITHUB_TOKEN` 由来のイベントから起きた run は自動実行されず、maintainer の承認を待つ。#681 で発火条件を直したが、あれは run が走る前提の修正なのでこの穴は塞げていない。

実害は 2 つある。バージョン上げが image ビルドを壊しても PR 上で検出できないこと、auto-merge が起動せず毎週人間の介入が要ること。どちらも「無人で依存を上げ続ける」という自動化の目的を損なう。

制約として、このリポジトリは public で、単一のメンテナしかいない。secret の定期ローテーションを回す運用体力は無い。

## Options considered

| 案                     | secret の寿命                            | 実際に渡る token の寿命      | 漏洩時の被害の窓     | 評価                                                                                                                                                                                                                                 |
| ---------------------- | ---------------------------------------- | ---------------------------- | -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 手動で Approve and run | なし                                     | —                            | なし                 | secret を増やさず CI は通せる。ただし週 1 回の手作業が残り、auto-merge による無人化を達成しない                                                                                                                                      |
| fine-grained PAT       | 最長 366 日、ローテーション必須          | PAT 本体（最長 366 日）      | 失効または気付くまで | 「無期限にして漏洩時の被害を永続させる」か「期限を切って年 1 回のローテーションを背負う」かの二択になる。後者は期限切れが週次 bump の失敗としてしか現れず、`[Automation] Pipeline Digest` が週 1 回なので気付くまで最悪 2 週間かかる |
| GitHub App             | private key は無期限、ローテーション不要 | installation token（1 時間） | 最大 1 時間          | App の作成とインストールが初期コストとして要る。secret は App ID と private key の 2 つに増えるが、どちらも期限管理が不要                                                                                                            |

## Decision

`actions/create-github-app-token` で GitHub App の installation token を発行し、それを `peter-evans/create-pull-request` の `token` に渡す。App に与える権限は `contents: write` と `pull-requests: write` のみとし、インストール先はこのリポジトリだけに絞る。

決め手は、PAT が突きつける二択そのものを消せること。private key は失効しないのでローテーション運用が発生せず、一方で実際にワークフローへ渡る token は 1 時間で死ぬ。運用コストと漏洩時の被害の窓が同時に下がるのは、この案だけだった。

手動承認を採らないのは、CI を通すことが目的ではなく、bump を無人で回すことが目的だから。人間が週 1 回ボタンを押す前提の自動化は、押さなかった週に静かに止まる。

## Consequences

今後 workflow から bot identity が必要になったときは、PAT を新たに発行せずこの App を使い回す。権限が足りなければ App の permission を広げる。これを既定とし、fine-grained PAT は採らない。

`BUMP_APP_ID` と `BUMP_APP_PRIVATE_KEY` が未設定の場合、token 発行 step が失敗して週次 bump が丸ごと落ちる。失敗が見えるのは `[Automation] Pipeline Digest`（週 1 回）になる。

`peter-evans/create-pull-request` に token を渡す構造上、この action が侵害されれば token も漏れる。全 action の SHA pin を維持し、pin の bump 時は差分を確認する。

これまで完了していなかった CI が bump PR で一斉に動くため、導入直後は docker build（timeout 120 分）を含む潜在的な失敗が一度に表面化しうる。最初は `workflow_dispatch` で手動実行して確認する。

`.github/workflows/automation-deps-auto-merge.yml` の発火条件は `startsWith(github.head_ref, 'chore/bump-tool-versions')` なので、PR author が変わっても追従する。変更は不要。

関連: [#787](https://github.com/hodanov/my-pde/issues/787)
