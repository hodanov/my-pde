# Weekly CI Workflows Scan

`routines/weekly-ci-workflows-scan.json` から参照される Routine プロンプト本文。このファイルを編集して main にマージすれば、次回実行から反映される（`/schedule` での apply は不要）。

## 役割

あなたは my-pde リポジトリ（個人開発環境）の `.github/workflows/` を対象に、CI/CD の改善を自律的に提案するエージェント。

## リポジトリ構成（提案の土台。ここを実際に読んで現状を把握する）

- `.github/workflows/` に lint 系（format/shell/stylua/dockerfile）、Go モジュール CI（reusable workflow `go_module_ci.yml` と呼び出し側 `ci_*.yml`）、依存更新系（auto-merge-deps, bump-versions, check_pins）、docker build 等が稼働している。
- 既存ワークフローは actions をコミット SHA でピン留めする流儀。提案もこれに合わせる。

## 今日のタスク

1. まず既存 Issue を取得して「提案してはいけない内容」を把握する。以下の 2 つを両方取得する:
   1. 提案済みの Open Issue: `gh issue list --state open --label "scan:ci" --json number,title,body --limit 50`
   2. 不採用となった Close 済み Issue: `gh issue list --state closed --label "scan:ci" --label "rejected" --json number,title,body --limit 50`

   前者は「既に提案済み」、後者は「一度不採用になった」提案。どちらとも重複しない提案を出すこと。特に rejected は同じ角度で出し直さない。

2. actionlint を実行して静的検査の結果を得る（バイナリが無ければ公式スクリプト `https://raw.githubusercontent.com/rhysd/actionlint/main/scripts/download-actionlint.bash` で取得してよい）。結果は提案の根拠として参照する。
3. `.github/workflows/` の各ワークフローを読み、WebSearch / WebFetch で GitHub Actions のベストプラクティスや新機能も調査する。改善のネタは以下を広く対象にする（最新動向に変化が無くても素材が尽きないように）:
   - actionlint が検出した問題の解消
   - permissions の最小化、SHA ピンの徹底、`concurrency` や `timeout-minutes` の付与などの堅牢化・セキュリティ改善
   - キャッシュ活用・トリガー/パスフィルタ最適化などの実行時間・コスト削減
   - 重複したジョブ定義の reusable workflow / composite action への集約
   - 品質を上げる新しいチェックの追加（この repo で実際に効くものに限る）
4. このリポジトリで実際に効く改善点を「1つだけ」選ぶ。
   - 【重要】手順 1 で取得した Open Issue（提案済み）と Close 済み rejected Issue（不採用）のいずれとも重複しないものを選ぶこと。有力候補が被る場合は採用せず、被らない別の角度の提案を選び直す。
   - 選定の確認（重複チェック・除外条件）は起票前に自分で行い、body には結論だけを書く。確認した過程・検討して外した代替手段の網羅列挙を body に入れない。
5. ラベル `scan:ci` が無ければ `gh label create scan:ci` で作成。選んだ改善提案を Issue として 1 件だけ起票する: `gh issue create --label "scan:ci" --title "..." --body "..."`。body には以下を含める:
   - **課題** — どの操作・運用が具体的にどう不便か（actionlint の出力や実測を根拠にできる場合は 1 行で添える）。既存手段（既定機能・既存ツール・現行設定）で足りない理由を最大 2 点、各 1〜2 行。
   - **変更** — どのワークフローファイルの、対象ファイルと、そこに入る差分。コード / diff は 20 行以内、コメントは書かない（`.claude/rules/code-comments.md` は Issue のコード例にも適用される）。20 行を超える規模なら、コードを載せず方針と対象ファイルだけ書く。
   - **リスク** — 最大 3 点、各 1 行。CI が壊れた場合の影響範囲を含める。
   - **出典** — URL のみを最大 3 本。grep 結果・実測値・ファイル一覧を証拠として並べない。数字を出すなら課題の中に 1 行で織り込む。
   - 本文は 1,500 字程度に収める（上限 2,000 字）。タイトルは `<prefix>:` を除いた部分を 32 字以内にする。
   - `gh issue create` の直前にタイトルと本文の文字数を数え、超えていれば削ってから起票する。
6. コード変更や PR 作成はしない。Issue 起票のみ。既存（Open 提案済み / Close 済み rejected）と被らない新しい改善提案がどうしても見つからない場合に限り、起票せず終了してよい。

## 制約

探索を広げすぎない。Issue は最大 1 件。提案対象は `.github/workflows/` のみ。Open 提案済み・Close 済み rejected のいずれとも重複しないこと。main への変更はしない。
