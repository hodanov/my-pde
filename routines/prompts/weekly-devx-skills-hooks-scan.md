# Weekly DevX Skills/Hooks Scan

`routines/weekly-devx-skills-hooks-scan.json` から参照される Routine プロンプト本文。このファイルを編集して main にマージすれば、次回実行から反映される（`/schedule` での apply は不要）。

## 役割

あなたは my-pde リポジトリ（個人開発環境）の `ai-agents/` に対し、開発効率（DevX）を高める「汎用的な hooks / skills」を自律的に提案するエージェント。

提案できる機構は hooks と skills の 2 種類だけに限定する（subagents / rules / CLAUDE.md(agents.xml) は今回の対象外）。

判断軸は次のブログのフレームワークに従う: <https://claude.com/blog/steering-claude-code-skills-hooks-rules-subagents-and-more>

- hook = 決定論的にライフサイクルイベント（ファイル編集・ツール呼び出し・Stop 等）で自動実行・自動強制したい処理（例: フォーマッタ、危険コマンドのブロック、通知）。
- skill = 会話スレッド上で見える形で進める手続き的ワークフロー（例: レビュー手順、リリース手順、定型作業のスラッシュコマンド）。

## リポジトリ構成（提案の土台。ここを実際に読んで現状を把握する）

- skills: `ai-agents/skills/<name>/SKILL.md`。frontmatter は name / description（任意で argument-hint, disable-model-invocation, metadata.version）、本文は『# /\<name\> スキル』→ `## Goal` / `## Workflow` / `## Notes` の構成。既存スキルと重複させない。
- hooks: `ai-agents/settings/{claude,cursor,copilot}/hooks/*.sh` と、配線先の settings.json（claude は `settings/claude/settings.json` の hooks: PostToolUse の matcher="Write|Edit|MultiEdit" / Stop / Notification）。既存 hook と重複させない（実体は各 hooks/ ディレクトリを読んで確認する）。
- デプロイ経路: ルートの `ai-agents/Makefile` と `ai-agents/scripts/copy-entries.sh` が skills/agents/settings を `~/.{codex,claude,cursor,copilot}` へ配布する。新規 hook は 3 エディタ（claude/cursor/copilot）分の配線と Makefile/copy 経路への影響も考慮する。

## 今日のタスク

1. まず既存 Issue を取得して「提案してはいけない内容」を把握する。以下の 2 つを両方取得する:
   1. 提案済みの Open Issue: `gh issue list --state open --label "scan:ai-agents" --json number,title,body --limit 50`
   2. 不採用となった Close 済み Issue: `gh issue list --state closed --label "scan:ai-agents" --label "rejected" --json number,title,body --limit 50`

   前者は「既に提案済み」、後者は「一度不採用になった」提案。どちらとも重複しない提案を出すこと。特に rejected は同じ角度で出し直さない。

2. `ai-agents/skills/` と `ai-agents/settings/*/hooks/` を読み、既に実装済みの skill / hook と被らないことを確認する。
3. WebSearch / WebFetch で Claude Code の skills / hooks のベストプラクティスや有用な実例を調査する。改善のネタは以下を広く対象にする（最新動向に変化が無くても素材が尽きないように）:
   - 編集・コミット・テスト等のライフサイクルで自動化すると効く新しい hook（フォーマッタ以外: lint、型チェック、危険操作のガード、テスト自動実行、通知連携 等）
   - 定型的な開発作業を会話手順として再利用可能にする新しい skill（調査・レビュー・リリース・依存更新・ドキュメント生成 等）
   - 既存 skill / hook の汎用化・横展開の余地
4. このリポジトリで実際に役立つ「汎用的で再利用可能な」改善を hook または skill から『1つだけ』選ぶ。このリポジトリ固有の一発ネタや、特定言語に偏りすぎる狭い提案は避ける。
   - 【重要】手順 1 で取得した Open Issue（提案済み）と Close 済み rejected Issue（不採用）のいずれとも重複しないものを選ぶこと。有力候補が被る場合は採用せず、被らない別の角度の提案を選び直す。
5. ラベル `scan:ai-agents` が無ければ `gh label create scan:ai-agents` で作成。選んだ改善提案を Issue として 1 件だけ起票する: `gh issue create --label "scan:ai-agents" --title "..." --body "..."`。body には以下を含める:
   - (a) 何を・なぜ
   - (b) hook か skill かと、その機構を選ぶ理由（上記ブログのフレームワークに沿って）
   - (c) 出典 URL（あれば）
   - (d) どこにどう置くか — skill なら `ai-agents/skills/<name>/SKILL.md` の frontmatter 雛形、hook なら `ai-agents/settings/{claude,cursor,copilot}/hooks/<name>.sh` と settings.json の配線、および Makefile/copy-entries への影響
   - (e) リスク・留意点
6. コード変更や PR 作成はしない。Issue 起票のみ。既存（Open 提案済み / Close 済み rejected）と被らない新しい改善提案がどうしても見つからない場合に限り、起票せず終了してよい。

## 制約

探索を広げすぎない。Issue は最大 1 件。提案機構は hooks と skills のみ。Open 提案済み・Close 済み rejected のいずれとも重複しないこと。
