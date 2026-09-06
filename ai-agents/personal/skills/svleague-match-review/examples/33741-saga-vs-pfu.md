# Example: SAGA久光 vs PFU (2026-04-18 / Match ID 33741)

`svleague-match-review` の動作確認用サンプル。SV リーグ WOMEN セミファイナル第1試合。

## 入力

- 試合 ID: `33741`
- 試合詳細 URL: <https://www.svleague.jp/ja/match/detail/33741>
- 応援チーム: SAGA久光
- 注目選手: （未指定の場合は候補提示 → ユーザー選択）

## 期待される抽出メタデータ

- 対戦カード: SAGA久光 vs PFU
- カテゴリ: `sv_women`
- 最終スコア: SAGA久光 1-3 PFU
- セット別スコア: 19-25, 25-19, 18-25, 17-25
- 大会: 2025-26 大同生命 SV.LEAGUE WOMEN セミファイナル (Match No.322)

## 期待される取得経路

| リソース  | URL                                                     | 想定                                                                |
| --------- | ------------------------------------------------------- | ------------------------------------------------------------------- |
| 試合詳細  | <https://www.svleague.jp/ja/match/detail/33741>         | 対戦カード / セット別スコア / カテゴリ判定                          |
| REPORT-A  | <https://www.svleague.jp/ja/sv_women/form/a/33741>      | メタデータ / スタメン / 交代 / コーチコメント                       |
| REPORT-B  | <https://www.svleague.jp/ja/sv_women/form/b/33741>      | 個人成績（速報版の可能性あり、冒頭に注記）                          |
| LiveScore | <https://www.svleague.jp/ja/sv_women/livescore/v/33741> | 3 タブ（ローテ / 試合経過 / 個人成績）。JS 依存で取得不可の場合あり |

## 期待される出力

- 保存先: `docs/svleague-match-review/2026-04-18_saga-vs-pfu.md`
- [templates/match-review.md](../templates/match-review.md) に沿ったレイアウト
- 視点: `team`（SAGA久光）/ `versus`（SAGA久光 vs PFU）/ `player`（ユーザー指定）
- 「選手に活かす情報」が各視点の小見出しに畳み込まれている
- データ鮮度（速報版 / LiveScore 取得可否）が本文冒頭に明記されている

## 確認ポイント

- カテゴリ判定（`sv_women`）が自動で通ること、失敗時にユーザー確認が走ること
- LiveScore は `--with-livescore` 未指定時はフェッチを試みず、付録に「(取得不可 / 未取得)」相当の注記が出ること
- 書き出し前にユーザー承認ステップが挟まること
- 保存先ディレクトリ `docs/svleague-match-review/` が無ければ自動作成されること
- データ鮮度のメタデータ表記が `REPORT-B {速報版|確定版} (取得: YYYY-MM-DD HH:MM JST) / LiveScore {取得可|取得不可 (理由)}` のフォーマットに揃っていること

## バリアント例

- `/svleague-match-review 33741 --views team,versus`
  - 視点 3（player ズーム）と template の `## 視点 3` セクション、注目選手の確定ステップが省かれる
  - メタデータの「注目選手」行も出力に含めない
- `/svleague-match-review 33741 --with-livescore`
  - LiveScore を WebFetch で試行する。取得できれば付録の `### LiveScore 要点` にローテ・試合経過・個人成績の時系列を埋める
  - 失敗時は本サンプル同様 `(取得不可)` を残す
