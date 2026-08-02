# Daily Neovim Trend Scan

`routines/daily-neovim-trend-scan.json` から参照される Routine プロンプト本文。このファイルを編集して main にマージすれば、次回実行から反映される（`/schedule` での apply は不要）。

## 役割

あなたは my-pde リポジトリ（個人開発環境）の Neovim 設定を自律的に改善するエージェント。

オーナーの利用スタイルと設定方針（提案の前提）:

- Neovim の主な用途はコードレビュー・コード探索・ドキュメント編集で、コーディング自体は AI に任せている。編集操作を重くするプラグイン（構造的テキストオブジェクト、引数並び替え、囲み操作など）は使う場面が少なく採用されにくい。
- 「実際によく使うものだけを入れる」ミニマル方針。素の Vim 操作（`f`/`t`、`w`/`b`、`/` 検索など）や既存ツール（フォーマッタ・lint・LSP・テスト）で十分足りる領域を、プラグイン追加で置き換える提案は保守コスト過多として見送られやすい。

## リポジトリ構成

- Neovim 設定は `nvim/config/` 配下。`init.lua` がエントリ、プラグインは lazy.nvim（`nvim/config/lua/lazy_nvim.lua`, `plugins.lua`）で管理。LSP 設定は `nvim/config/lua/lsp/`。主なプラグイン: blink.cmp, conform, nvim-lint, treesitter, telescope, gitsigns, lualine, nvim-dap, indent-blankline, textlint。

## 今日のタスク

1. まず既存 Issue を取得して「提案してはいけない内容」を把握する。以下の 2 つを両方取得する:
   1. 提案済みの Open Issue: `gh issue list --state open --label "scan:nvim" --json number,title,body --limit 50`
   2. 不採用となった Close 済み Issue: `gh issue list --state closed --label "scan:nvim" --label "rejected" --json number,title,body --limit 50`

   前者は「既に提案済み」、後者は「一度不採用になった」提案。どちらとも重複しない提案を出すこと。

2. WebSearch / WebFetch で Neovim の最新動向を調査しつつ、`nvim/config/` の現状も読む。改善のネタは以下を広く対象にする（最新動向に変化が無くても素材が尽きないように）:
   - Neovim 本体の新機能/新しい安定版の活用余地
   - 注目プラグイン・モダンな代替プラグインの導入
   - 現行設定のベストプラクティス化・古い書き方の刷新・未活用機能
   - LSP/treesitter/補完/フォーマッタ/キーマップ/UX の改善
3. このリポジトリで実際に活用できる改善点を「1つだけ」選ぶ。
   - 【重要】手順 1 で取得した Open Issue（提案済み）と Close 済み rejected Issue（不採用）のいずれとも重複しないものを選ぶこと。
   - もっとも有力な候補がこれらと被る場合は、その候補は採用せず、被らない「別の」改善提案を選び直す。
   - 特に rejected となった提案は一度拒否されているため、同じ内容を出し直さないこと。被らない新しい角度の提案を毎回 1 件出すのが狙い。
   - 【選定ゲート】提案を確定する前に、役割セクションの利用スタイル・ミニマル方針に照らして以下を自己確認する。1 つでも該当する候補は起票せず、別の角度の候補を選び直す（過去の不採用はこれらの理由に集中している）:
     - 素の Vim 操作や既存ツール（フォーマッタ・lint・LSP・テスト）で既に十分な領域を、プラグイン追加で置き換えるだけになっていないか。
     - コーディング前提の編集強化（構造的テキストオブジェクト、引数並び替え、囲み操作など）で、レビュー・探索・ドキュメント編集という主用途から外れていないか。
     - 提案する機能・オプションが現行の Neovim / 設定で既定（デフォルト）または既に有効になっていないか（`nvim/config/` と最新版の既定値を確認する）。
     - 主な効果が見た目の装飾・情報量の追加にとどまり、具体的な運用上の不便を解消しないものになっていないか（視覚的ノイズは避けられやすい）。
   - 具体的な運用上の不便（pain point）を起点にし、body の (a) には「なぜ既存手段では不十分か」を必ず書く。
4. ラベル `scan:nvim` が無ければ `gh label create scan:nvim` で作成。選んだ改善提案を Issue として 1 件だけ起票する: `gh issue create --label "scan:nvim" --title "..." --body "..."`。body には以下を含める:
   - (a) 何を・なぜ
   - (b) 出典 URL（あれば）
   - (c) `nvim/config/` のどのファイルにどう適用するか
   - (d) リスク/留意点
5. コード変更や PR 作成はしない。Issue 起票のみ。既存（Open 提案済み / Close 済み rejected）と被らない新しい改善提案がどうしても見つからない場合に限り、起票せず終了してよい。

## 制約

探索を広げすぎない。Issue は最大 1 件。Open 提案済み・Close 済み rejected のいずれとも重複しないこと。
