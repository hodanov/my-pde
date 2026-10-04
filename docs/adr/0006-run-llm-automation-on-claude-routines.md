# ADR-0006: Run LLM automation on Claude Routines instead of claude-code-action

- Status: Accepted (2026-06-18)

## Context

リポジトリ全体の改善提案を定期的に Issue として起票する仕組みを、まず `anthropics/claude-code-action@v1` の cron workflow（`.github/workflows/weekly-improvement-scan.yml`）として作った。認証は従量課金を避けるため、Pro サブスクの OAuth トークン（`claude setup-token` で発行し、Secret `CLAUDE_CODE_OAUTH_TOKEN` に登録）にした。経緯と設計は [`docs/plan/2026-06-15_autonomous-improvement-scan.md`](../plan/2026-06-15_autonomous-improvement-scan.md) にある。

試運転では 2 つ問題が出た。1 つは `--max-turns` を超えて実行が途中で止まったこと。もう 1 つは、OAuth トークンに期限があるため、発行と Secret の差し替えを手で回し続ける必要があること。

制約として、このリポジトリはメンテナが 1 人で、期限付き secret のローテーションを回す運用体力は無い（[ADR-0002](0002-use-github-app-for-workflow-bot-auth.md) と同じ前提）。

2026-10-04 に、Routine が作った PR #860 に `.claude/rules/code-comments.md` 違反が残っていたことから、ルール準拠をチェックする LLM レビュー CI を再び検討した。この判断が plan 文書の注記にしか残っていなかったため、同じ案がもう一度出てきた。

## Options considered

| 案                                       | 不採用の理由                                                                                                             |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| claude-code-action + OAuth トークン      | `--max-turns` 超過で止まった。トークンの発行と期限管理を手で回す必要があり、期限切れは workflow の失敗としてしか見えない |
| claude-code-action + API キー            | トークン管理の問題は消えるが、従量課金になる。Pro サブスクの範囲で回すという前提を崩す                                   |
| claude-code-action による PR レビュー CI | 2026-10-04 に検討した。人が作った PR も対象にできるが、上の OAuth トークンの管理の手間がそのまま残る                     |

## Decision

LLM を使う自動化（定期スキャン、Issue の PR 化、PR のケア）は claude.ai の Routine で動かす。定義の正は `routines/` に置く。認証は Routine 側（claude.ai）が持つから、リポジトリに Claude 用の secret は置かない。

Routine が作る変更の品質も、CI を足さずに Routine の手順の中で担保する。ルール準拠は、PR Bot のプロンプトに「PR を作る前に diff を `.claude/rules/` に照らして見直す」工程を足して守らせる（#867）。

## Consequences

`.github/workflows/weekly-improvement-scan.yml` は削除済みで、`CLAUDE_CODE_OAUTH_TOKEN` も使わない。今後 LLM による自動チェックが欲しくなったら、まず `routines/prompts/*.md` に工程を足せないかを考える。

人が作った PR には、自動の LLM レビューが付かない。ローカル作業では `dev-workflow` スキルと path-scoped rules で担保する。

Routine の実行は、対話作業と同じサブスクの使用量枠を消費する。Routine を増やすときは、頻度と 1 回あたりの対象範囲を絞る。

Routine 内のセルフレビューは、作った本人が見直すだけなので見落としが残りうる。漏れが続くなら、まず `routines/prompts/weekly-pr-care-bot.md` に後追いのチェックを足す。CI に戻すなら、この ADR を supersede する新しい ADR で、トークン管理の問題をどう解いたかを示す。
