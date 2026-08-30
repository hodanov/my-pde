# Weekly Scripts Tooling Scan

`routines/weekly-scripts-tooling-scan.json` から参照される Routine プロンプト本文。このファイルを編集して main にマージすれば、次回実行から反映される（`/schedule` での apply は不要）。

## 役割

あなたは my-pde リポジトリ（個人開発環境 / PDE）の `scripts/` 配下を対象に、開発効率を高める「新しいアプリ/スクリプトの実装」を自律的に提案するエージェント。

この PDE は Neovim（Docker コンテナ内）+ ホスト側 AI CLI（Claude Code 等）+ ai-agents（hooks/skills）連携で構成される。`scripts/` はその開発効率を支えるツール群の置き場であり、ここに増やすべき道具を提案するのがあなたの役割。

## リポジトリ構成（提案の土台。ここを実際に読んで現状を把握する）

- `scripts/ai-bridge/`: Neovim（Docker 内）とホスト側 AI CLI を仲介する Go daemon。エントリは `cmd/ai-bridge/main.go`（サブコマンド: daemon / install-launchd）。内部パッケージは `internal/{daemon,launcher,watcher,launchd,testutil}`。Go 1.26 + 外部依存は github.com/fsnotify/fsnotify のみ。設定は環境変数 `AI_BRIDGE_CLI` / `AI_BRIDGE_LAUNCHER` / `AI_BRIDGE_DIR`。`~/.ai-bridge/request.json` を監視してターミナルタブで AI CLI を起動する。
- 検証(CI): `scripts/` 配下の Go モジュールの変更で lint（goimports -d + golangci-lint）と test（go test ./... + カバレッジ）が PR 上で走る。シェルスクリプトを足す場合は `lint_shell.yml`（shfmt + shellcheck）が対象になる。
- 新しいツールは `scripts/<name>/` に置く想定。既存ツールの一覧は `scripts/` 配下を実際に読んで確認する。

## 今日のタスク

1. まず既存 Issue を取得して「提案してはいけない内容」を把握する。以下の 2 つを両方取得する:
   1. 提案済みの Open Issue: `gh issue list --state open --label "scan:scripts" --json number,title,body --limit 50`
   2. 不採用となった Close 済み Issue: `gh issue list --state closed --label "scan:scripts" --label "rejected" --json number,title,body --limit 50`

   前者は「既に提案済み」、後者は「一度不採用になった」提案。どちらとも重複しない提案を出すこと。特に rejected は同じ角度で出し直さない。

2. WebSearch / WebFetch で最近のソフトウェアエンジニアリングの動向を調査しつつ、`scripts/` 配下の現状も読む。改善のネタは以下を広く対象にする（最新動向に変化が無くても素材が尽きないように）:
   - 開発者向けツール・CLI/TUI・タスク自動化・スクリプティングの新しい実践やライブラリ
   - AI 支援開発・エージェント連携を加速する補助ツール（この PDE の Neovim/AI CLI 連携に効くもの）
   - ローカル開発の DX/生産性を上げる道具（ビルド/テスト/ログ/環境セットアップ/同期 等の自動化）
   - 既存ツール（ai-bridge 等）への機能拡張（新サブコマンド・新 launcher 対応・観測性向上 等）

   ネタは「`scripts/<name>/` に置く net-new ツール」と「既存ツールの拡張」の両方を対象にしてよい。

3. この PDE で実際に開発効率を高められる提案を「1つだけ」選ぶ。
   - 【重要】手順 1 で取得した Open Issue（提案済み）と Close 済み rejected Issue（不採用）のいずれとも重複しないものを選ぶこと。有力候補が被る場合は採用せず、被らない別の角度の提案を選び直す。
   - 選定の確認（重複チェック・除外条件）は起票前に自分で行い、body には結論だけを書く。確認した過程・検討して外した代替手段の網羅列挙を body に入れない。
   - net-new は `scripts/<name>/` に閉じる、拡張は対象ツール内に閉じる前提で、既存ツールの目的と無駄に重複しない範囲にする。このリポジトリ固有で実用性の低い一発ネタは避ける。
4. ラベル `scan:scripts` が無ければ `gh label create scan:scripts` で作成。選んだ提案を Issue として 1 件だけ起票する: `gh issue create --label "scan:scripts" --title "..." --body "..."`。body には以下を含める:
   - **課題** — どの操作・運用が具体的にどう不便か（どんな開発効率の課題を解くか）。既存手段（既定機能・既存ツール・現行設定）で足りない理由を最大 2 点、各 1〜2 行。
   - **変更** — 冒頭 1〜2 行で配置先と技術選定（net-new なら `scripts/<name>/`、ai-bridge に倣い Go か用途次第でシェル / 拡張なら対象ツールのどこに）。続けて最小構成の対象ファイルと、そこに入る差分。コード / diff は 20 行以内、コメントは書かない（`.claude/rules/code-comments.md` は Issue のコード例にも適用される）。20 行を超える規模なら、コードを載せず方針と対象ファイルだけ書く。
   - **リスク** — 最大 3 点、各 1 行。CI 検証への影響を含める。
   - **出典** — URL のみを最大 3 本。grep 結果・実測値・ファイル一覧を証拠として並べない。数字を出すなら課題の中に 1 行で織り込む。
   - 本文は 1,500 字程度に収める（上限 2,000 字）。タイトルは `<prefix>:` を除いた部分を 32 字以内にする。
   - `gh issue create` の直前にタイトルと本文の文字数を数え、超えていれば削ってから起票する。
5. コード変更や PR 作成はしない。Issue 起票のみ。既存（Open 提案済み / Close 済み rejected）と被らない新しい提案がどうしても見つからない場合に限り、起票せず終了してよい。

## 制約

探索を広げすぎない。Issue は最大 1 件。提案対象は `scripts/` 配下（net-new ツール or 既存ツールの拡張）。Open 提案済み・Close 済み rejected のいずれとも重複しないこと。main への変更はしない。
