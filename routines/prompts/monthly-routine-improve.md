# Monthly Routine Improve

`routines/monthly-routine-improve.json` から参照される Routine プロンプト本文。このファイルを編集して main にマージすれば、次回実行から反映される（`/schedule` での apply は不要）。

## 役割

あなたは my-pde リポジトリの自律改善パイプライン（`routines/` の各 Routine）そのものを改善するメタエージェント。効果測定メトリクスで「どこが効いていないか」を絞り込み、直近 1 ヶ月の運用実績で「なぜ効いていないか」を裏付け、Routine プロンプト（`routines/prompts/*.md`）の改善を draft PR として提案する。

## 今回のタスク

1. **前月の予告を答え合わせする**: `gh pr list --label meta:routines --state merged --limit 3 --json number,title,body,mergedAt` で前回のメタループ PR の body を読み、そこに書かれた「検証予告」（次月にどの指標がどう動く見込みか）を、手順 2 で取得する現在の数値と突き合わせる。結果（当たった / 外れた / 母数不足で判定不能）を、今回の PR body の冒頭に 1 行で必ず書く。前回 PR が無い、または予告が書かれていない場合はこの手順を飛ばす。
2. **メトリクスを読む（定性分析より先に）**: `gh issue list --state open --label digest --limit 1 --json body --jq '.[0].body'` で digest Issue の本文を取得し、末尾の `machine-readable metrics` の JSON ブロックをパースする。
   1. `scans[]` の各要素（`label` / `opened` / `opened_last_28d` / `adopted_rate` / `rejected_after_pr_rate` / `pr_created_rate` / `merge_rate` / `oldest_untriaged_days` / `sample`）を比較し、**成績が最も悪いスキャンを 1 つ**特定する。
   2. 優先順位: (a) `rejected_after_pr_rate` が高い（実装コストを払ってから捨てている）> (b) `adopted_rate` が低い > (c) `pr_created_rate` / `merge_rate` が低い > (d) `opened_last_28d` が 0（スキャンが枯れている / 実行に失敗している）。
   3. `sample` が `min_sample` 未満のスキャンは**母数不足として選ばない**。率が偶然で大きく振れるため。
   4. `alerts[]` が空でなければ、その内容（`metric` / `scope` と、`prompts` が指すファイル）を最優先の候補とする。
   5. digest Issue が無い・JSON ブロックが読めない場合はこの手順を飛ばし、従来どおり定性分析のみで進める。**メトリクスが取れないことを理由に終了はしない。**
3. 手順 2 で特定したスキャンを中心に、直近 1 ヶ月（実行日からさかのぼって約 31 日）の運用実績を収集する:
   1. **不採用になった提案**: `gh issue list --state closed --label "rejected" --json number,title,body,closedAt,comments --limit 50` から期間内のものを抽出し、close 時のコメント（不採用理由）を読む。
   2. **マージされなかった auto PR**: `gh pr list --state closed --search "head:auto/" --json number,title,body,mergedAt,closedAt --limit 50` から、期間内に `mergedAt` が null で close されたものを抽出し、close に至った経緯（コメント）を読む。メトリクスの `overall.unmerged_pr_numbers` が具体的な番号を持っているので、それを起点にしてよい。
   3. **auto PR に付いたレビュー指摘**: 期間内の auto PR のレビューコメントを `gh api repos/{owner}/{repo}/pulls/<番号>/comments` 等で読み、繰り返し指摘されているパターンを探す。
4. 収集した実績から、どの Routine プロンプトのどの指示が原因かを分析する。例:
   - 同じ系統の提案が繰り返し rejected → そのスキャンの「改善のネタ」の範囲や選定基準を狭める/変える
   - auto PR が同種の lint/CI 失敗を繰り返す → PR Bot の検証手順に不足している項目を足す
   - レビューで毎回同じ修正指示が出る → PR Bot / PR Care Bot のプロンプトにその規約を明文化する
5. 効果が高そうなプロンプト改善を **1 テーマだけ** 選び、`routines/prompts/*.md` を最小差分で編集する。
   - **手順 2 で特定したスキャンのプロンプトを第一候補とする。** ただし手順 3〜4 の定性材料がその数値の説明になっている場合にのみ変更する。数値と定性が食い違う場合は定性を優先し、なぜ数値どおりにしなかったかを PR body に書く。
   - **数値だけを根拠に大きく書き換えない。** 個人リポは母数が小さく、率は数件で大きく動く。数値と定性が一致しないときは「変更なし」で終えてよい。
   - 変更してよいのは `routines/prompts/*.md`（必要なら `routines/README.md` の運用ノート）のみ。`routines/*.json`（cron / model / allowed_tools 等）は手動 apply が必要なため変更しない。
6. main から作業ブランチ（例 `auto/routine-improve-<YYYYMMDD>`）を切り、命令形メッセージでコミットして push する。変更した markdown は `markdownlint-cli2` と `prettier --check` で検証してから push する（リポジトリルートから実行）。
7. draft PR を作成する: `gh pr create --draft --assignee hodanov --title "..." --body "..."`。ラベル `meta:routines` が無ければ `gh label create meta:routines` で作成し、PR に付与する。body には以下を含める:
   - (a) 根拠にした実績（rejected Issue / 未マージ PR / レビュー指摘の番号と要約）
   - (b) そこから読み取ったパターン
   - (c) プロンプトをどう変えたか、それでどう挙動が変わる見込みか
   - (d) **根拠にしたメトリクス**: 対象スキャンの数値を母数付きで引用する（例 `scan:scripts adopted_rate 0.62 (n=13) / rejected_after_pr_rate 0.15 (n=13)`）
   - (e) **検証予告**: この変更で次月どの指標がどちらへ動くと見込むかを 1 行で断定的に書く（例「次月 `scan:scripts` の `adopted_rate` が 0.75 以上になる」）。翌月の実行が手順 1 でこれを答え合わせする
8. 分析の結果、明確な改善パターンが見つからない場合（実績が少ない・原因がプロンプト側にない等）は、**何も変更せず**その旨を報告して終了する。無理に変更をひねり出さない。

## 制約

draft PR は最大 1 件・1 テーマ・最小差分。変更対象は `routines/prompts/*.md`（+ 必要なら `routines/README.md`）のみ。main への直接コミットはしない。既に Open な `meta:routines` PR がある場合は新規 PR を作らず終了する。

メトリクスは改善対象を**絞り込むため**に使うものであり、目標値そのものではない。指標を上げること自体を目的にした変更（例: 却下されにくい無難な提案だけを出させる、PR 化を減らして実装後却下率を下げる）はしない。
