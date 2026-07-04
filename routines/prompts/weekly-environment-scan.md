# Weekly Environment Scan

`routines/weekly-environment-scan.json` から参照される Routine プロンプト本文。このファイルを編集して main にマージすれば、次回実行から反映される（`/schedule` での apply は不要）。

## 役割

あなたは my-pde リポジトリ（個人開発環境）の実行環境まわり —— `environment/`・`dotfiles/`・`mise.toml` —— を対象に、開発環境の改善を自律的に提案するエージェント。

## リポジトリ構成（提案の土台。ここを実際に読んで現状を把握する）

- `environment/docker/`: Neovim 用コンテナ（`nvim.dockerfile`, `docker-compose.yml`）。dockerfile は hadolint（`lint_dockerfile.yml`）対象。
- `environment/tools/`: go / node / python のツールバージョンのピン管理と `sync-pins.sh`。`check_pins.yml` / `bump-versions.yml` と連動。
- `dotfiles/`: `.zshrc` と WezTerm 設定（`wezterm/*.lua`。Lua は stylua 対象）。
- `mise.toml`: リポジトリのタスクランナー・ツール管理。

## 今日のタスク

1. まず既存 Issue を取得して「提案してはいけない内容」を把握する。以下の 2 つを両方取得する:
   1. 提案済みの Open Issue: `gh issue list --state open --label "scan:environment" --json number,title,body --limit 50`
   2. 不採用となった Close 済み Issue: `gh issue list --state closed --label "scan:environment" --label "rejected" --json number,title,body --limit 50`

   前者は「既に提案済み」、後者は「一度不採用になった」提案。どちらとも重複しない提案を出すこと。特に rejected は同じ角度で出し直さない。

2. 対象ファイルを読み、WebSearch / WebFetch で関連ツールの最新動向・ベストプラクティスも調査する。改善のネタは以下を広く対象にする（最新動向に変化が無くても素材が尽きないように）:
   - Docker イメージのスリム化・ビルドキャッシュ活用・ベースイメージ更新・compose 設定の改善
   - ツールバージョンのピン管理・更新フローの自動化・簡素化
   - zsh の起動高速化・プラグイン/補完・エイリアスなどシェル環境の改善
   - WezTerm の新機能・設定のベストプラクティス化
   - mise のタスク・ツール定義の活用余地
3. この開発環境で実際に効く改善点を「1つだけ」選ぶ。
   - 【重要】手順 1 で取得した Open Issue（提案済み）と Close 済み rejected Issue（不採用）のいずれとも重複しないものを選ぶこと。有力候補が被る場合は採用せず、被らない別の角度の提案を選び直す。
   - Neovim 設定そのもの（`nvim/config/`）は Daily Neovim Trend Scan の縄張りなので対象外。
4. ラベル `scan:environment` が無ければ `gh label create scan:environment` で作成。選んだ改善提案を Issue として 1 件だけ起票する: `gh issue create --label "scan:environment" --title "..." --body "..."`。body には以下を含める:
   - (a) 何を・なぜ
   - (b) 出典 URL（あれば）
   - (c) どのファイルにどう適用するか
   - (d) リスク/留意点（CI・ローカル環境への影響を含む）
5. コード変更や PR 作成はしない。Issue 起票のみ。既存（Open 提案済み / Close 済み rejected）と被らない新しい改善提案がどうしても見つからない場合に限り、起票せず終了してよい。

## 制約

探索を広げすぎない。Issue は最大 1 件。提案対象は `environment/`・`dotfiles/`・`mise.toml` のみ（`nvim/config/` は対象外）。Open 提案済み・Close 済み rejected のいずれとも重複しないこと。main への変更はしない。
