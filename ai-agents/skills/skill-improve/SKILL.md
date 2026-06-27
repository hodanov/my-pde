---
name: skill-improve
description: >-
  蓄積された observations を分析し、SKILL.md への改善提案を生成する。
  Observe → Inspect → Amend → Evaluate ループの Inspect/Amend フェーズ。
  `--apply` フラグで承認後の適用も可能。
argument-hint: "<スキル名|all> [--apply]"
metadata:
  version: 1
---

# /skill-improve スキル

## Goal

対象スキルの observations を分析し、SKILL.md の具体的な改善提案（amendment）を生成する。`--apply` 指定時はユーザー承認のうえ適用する。

## Workflow

### Step 1: 引数パース

`$ARGUMENTS` を以下のように解釈する:

- **第1引数**: スキル名 または `all`（全スキル横断分析）
- **`--apply` フラグ**: 提案を承認後に適用するかどうか（省略時は提案のみ）

引数なしの場合、observations を持つスキル一覧を表示し選択を促す。

### Step 2: observations 読み込み

対象スキルの `ai-agents/skills/<スキル名>/observations/YYYY-MM-DD_NNN_obs.md` を全て読み込む。

- observations がない場合、その旨を通知して終了
- `all` の場合、全スキルの `observations/*_obs.md` を走査（`amendments/` は除外）

### Step 3: パターン分析

以下の 4 観点で observations を分析する:

| 観点               | 分析内容                                           |
| ------------------ | -------------------------------------------------- |
| 失敗頻度           | 直近 10 回中の failure / partial の割合            |
| 再発パターン       | 同じ「問題」の繰り返し（類似キーワードの出現頻度） |
| コンテキスト依存   | 特定条件（言語、規模、リポ種別）での失敗集中       |
| フィードバック傾向 | 共通する不満や要望のキーワード                     |

### Step 4: 前回 amendment の効果評価（Evaluate フェーズ）

`ai-agents/skills/<スキル名>/observations/amendments/` に過去の amendment がある場合:

1. amendment 適用日をファイル名の `YYYY-MM-DD` 部分から特定（複数ある場合はファイル名の辞書順で最新を選択）
2. 適用前後の failure/partial 率を比較
3. amendment 後に新たな失敗パターンが出現していないか確認
4. 効果が見られない or 悪化している場合、ロールバックを提案

`all` モードでは各スキルの直近 amendment の効果のみをサマリ一覧表示する。個別スキルモードの場合のみ詳細評価を行う。

この評価結果を分析レポートの冒頭に記載する。

### Step 5: 改善提案の生成

分析結果に基づき、`ai-agents/skills/skill-improve/template.md` のフォーマットで amendment proposal を生成する。

提案には以下を含める:

- SKILL.md の具体的な変更箇所（diff 形式推奨）
- 変更理由（どの observations がエビデンスか）
- 期待される効果
- リスク（副作用の可能性）

提案内容をユーザーに表示する。

### Step 6: 適用（`--apply` フラグがある場合のみ）

1. ユーザーに提案内容を表示し、**明示的な承認** を求める
2. 承認されたら SKILL.md を編集
3. SKILL.md の `version:` フィールドをインクリメント（なければ `metadata` 配下に `version: 1` を追加。 `metadata`もない場合は追加する）
4. amendment 記録を `ai-agents/skills/<スキル名>/observations/amendments/YYYY-MM-DD_NNN_amendment.md` に保存（NNN は同日の連番、001 から開始。既存ファイルと重複しないようインクリメントする）
5. 変更内容をユーザーに表示

承認されなかった場合は適用せず終了。

## Notes

- 自動書き換えは絶対にしない。常にユーザー承認を経る
- `all` モードは横断的な傾向を見るためのもの。個別スキルの改善提案は各スキルに対して個別に行う
- observations が 5 件未満の場合、分析の信頼度が低い旨を注記する
- git 履歴が第一のバージョン管理。amendment 適用は専用コミットで記録することを推奨する
