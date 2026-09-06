---
paths:
  - ".github/workflows/**"
---

# GitHub Actions workflow naming rules

ファイル名と表示名から役割を判断できるようにする。
既存workflowは2026-09-06に本規則へ一括で統一済み。以後は、workflowの責務が変わったときだけ改名し、
規則の書き換えだけを理由に再度一括改名しない。

## 核となる原則

ファイル名のカテゴリ接頭辞と、workflow表示名の `[Category]` を一対一で対応させる。
ファイル名は小文字kebab-case、表示名は英語Title Caseで統一する。

## カテゴリ

| ファイル接頭辞 | 表示名         | 用途                                             |
| -------------- | -------------- | ------------------------------------------------ |
| `ci-`          | `[CI]`         | pull requestのlint、test、buildなどの検証        |
| `check-`       | `[Check]`      | 生成物の同期確認やリポジトリ状態のガード         |
| `automation-`  | `[Automation]` | 保守・更新の自動実行（定期・手動・イベント駆動） |

必要になっていないカテゴリを先取りで追加しない。

## ファイル名

- 小文字kebab-caseを使う。
- 拡張子は既存workflowに合わせて `.yml` を使う。
- `<category>-<area>[-<action>].yml` の順で、対象と動作が分かる名前にする。
- `<action>` は、カテゴリ接頭辞だけでは動作が一意に定まらないときに付ける。
  同じ `ci-` でも動作が分かれる場合は付け（`ci-format-lint.yml` / `ci-docker-build.yml`）、
  対象を挙げれば動作が定まる場合は省く（Goモジュールの検証は `ci-<module>.yml`）。
- reusable workflow（`on.workflow_call`）も、呼び出し元と同じカテゴリ接頭辞を使う（`ci-go-module.yml`）。
- workflowを改名するときは、そのファイル名を参照するtrigger、script、文書も同時に更新する。
- ファイル名を組み立てる生成器（`scripts/scaffold`）がある場合は、そのコードとテストも同時に更新する。

## Workflow表示名

- `name: "[Category] Title Case"` の形式を使う。
- `[` で始まるYAML scalarは、構文上の曖昧さを避けるためダブルクオートで囲む。
- 本文は英語Title Caseとし、絵文字や日本語を使わない。
- ファイル名のカテゴリと表示名のカテゴリを一致させる。

## Job名

- 既存のjob IDやjob表示名を、命名統一だけを理由に変更しない。
- branch protectionの必須ステータスチェックとして参照されている可能性を確認する。
- 変更が必要なら、GitHub側の保護設定と同じ変更単位で扱う。

## Paths

- `on.pull_request.paths` に自workflowのパスがある場合、ファイル改名時に追従させる。
- 必須ステータスチェックに設定されたworkflowは、`on.pull_request.paths` でtrigger自体を絞らない。
- 必須workflowの実行対象を差分で絞る場合は、triggerを起動した上でjobまたはstepを条件分岐させる。
- path filterを変更するときは、対象差分と対象外差分のどちらでも必須checkが完了することを確認する。

## 判断チェック

1. workflowの責務は既存カテゴリのどれに当たるか。
2. ファイル名から対象と動作が分かるか。
3. ファイル接頭辞と表示名のカテゴリが一致しているか。
4. 表示名をダブルクオートで囲んでいるか。
5. 改名対象を参照するpathや文書を更新したか。
6. ファイル名を組み立てるコード（`scripts/scaffold` 等）が規則と一致しているか。
7. triggerの絞り込みが必須checkをpendingのまま残さないか。
