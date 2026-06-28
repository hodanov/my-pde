# Plan: skill-observe を Stop hook で自動化する（方向A）

## Context

**なぜやるか**: `skill-observe` / `skill-improve` は改善ループ（Observe → Inspect →
Amend → Evaluate）のために用意したが、実運用で回っていない。原因は **Observe の摩擦**:
スキルを使うたびに手動で `/skill-observe` を起動し、結果と問題を毎回言語化する必要があり、
面倒で続かない。結果として observation が溜まらず（現状 `credential-leak-prevention` の
1 件のみ）、`skill-improve` も分析対象がなく回らない。

「スキル使用後に毎回・決定論的に走る記録のトリガー」はまさに hook の領域（手動 skill
ではなく）。本変更で **Observe の起動を hook に自動化** し、observation の中身は会話文脈を
持つ Claude がその場で自動生成する。これで「トリガーの手間」と「内容を伝える手間」の両方を
ゼロにする。`skill-improve`（Inspect/Amend）は人間承認が要る設計のため**手動のまま**。

**決定事項（ユーザー確認済み）**:

- 記録の主体: **Claude が自動で全部書く**（hook は検知して `decision:block` で差し戻し、
  Claude が success/partial/failure 判定＋6フィールドを埋めて書き出す。ユーザー操作なし）。
- 記録対象: **全結果（success 含む）**・`ai-agents/skills/` 配下の全リポジトリスキル。
  ただし `skill-observe` / `skill-improve` 自身は除外。

## 動作確認済みの前提

- Claude Code の Stop hook は stdin で `transcript_path` / `session_id` / `cwd` /
  `hook_event_name` を受け取る（公式ドキュメントで確認）。
- transcript は JSONL。スキル起動は `tool_use` エントリ `name=="Skill"` として記録され、
  `.input.skill` にスキル名が入る（実 transcript で確認済み）。
  抽出: `jq -r '.message.content[]? | select(.type=="tool_use" and .name=="Skill") | .input.skill'`
- Stop hook は `{"decision":"block","reason":"..."}` を stdout に返すと停止をブロックして
  `reason` を Claude に差し戻し、会話を継続させられる（公式で確認）。
- Stop hook は Claude Code 固有機能。Cursor/Copilot に Stop 相当はなく、既存の
  `lint-changed.sh` / `notify-macos.sh` も Claude 専用。**よって本 hook も Claude のみ**で、
  cursor/copilot 配下の変更は不要。

## 変更内容

### 1. 新規: `ai-agents/settings/claude/hooks/skill-observe-nudge.sh`

Stop hook。既存 `lint-changed.sh` / `notify-macos.sh` の bash 3.2 互換・`jq` 利用・graceful
exit のイディオムに合わせる。ロジック:

1. `INPUT=$(cat)`。`command -v jq` が無ければ `exit 0`。
2. `transcript_path` / `session_id` / `stop_hook_active` / `cwd` を `jq` で抽出。
3. **ループガード①**: `stop_hook_active == "true"` なら `exit 0`（継続中の再発火を抑止）。
4. `transcript_path` が空 or ファイル無しなら `exit 0`。
5. リポジトリルートを `git -C "$cwd" rev-parse --show-toplevel` で特定。`ai-agents/skills`
   が無ければ `exit 0`。
6. **対象スキル集合**を構築: `ai-agents/skills/*/SKILL.md` の親ディレクトリ名一覧から
   `skill-observe` / `skill-improve` を除外。
7. **使用スキル**を transcript から抽出（上記 jq）→ ユニーク化 → 対象スキル集合と積集合 =
   `used_skills`。
8. **セッション状態ファイル**: `${TMPDIR:-/tmp}/claude-skill-observe-nudge/<session_id>`。
   既に nudge 済みのスキルを読み、`pending = used_skills − 既 nudge` を計算。
9. `pending` が空なら `exit 0`。
10. **ループガード②（主防御）**: `pending` を状態ファイルに追記（`stop_hook_active` の有無に
    関わらず、同一セッション・同一スキルは二度と nudge しない）。
11. stdout に `{"decision":"block","reason":"<指示文>"}` を出力して `exit 0`。

`reason` の指示文（日本語）の要点:

- 「今セッションで次のリポジトリスキルを使用したが observation 未記録: `<pending一覧>`」
- 各スキルについて、直近の使用結果を会話文脈から `success`/`partial`/`failure` で判定し、
  `ai-agents/skills/<name>/observations/YYYY-MM-DD_NNN_obs.md` を
  **`skill-observe` SKILL.md の6フィールド形式**（タスク/スキル/結果/問題/フィードバック/
  コンテキスト）で作成せよ。`NNN` は当日連番、既存と重複しないよう採番。
- 当日同スキルの observation が既にあればスキップ可。
- **ユーザーへの確認は不要**。淡々と記録し、作成ファイルを1行で報告して終了せよ。
- markdownlint を通すため既存 observation ファイルの体裁に合わせること
  （Stop の `lint-changed.sh` が .md を検査するため）。

**ループ安全性**: ブロック → Claude が obs を書く → 再 Stop。このときガード①
（`stop_hook_active`）かガード②（状態ファイルに記録済みで `pending` 空）のいずれかで必ず
`exit 0`。二重防御で無限ループしない。別セッションでは `session_id` が変わり状態ファイルも
新規になるため、必要なら再度1回だけ nudge される（意図どおり）。

ファイルは `chmod +x` で実行権限を付与（`copy-entries.sh` は `cp -p` で権限を維持するため）。

### 2. 編集: `ai-agents/settings/claude/settings.json`

`hooks.Stop[0].hooks` 配列に新 hook を追加（`notify-macos.sh` より前に置き、ブロック判定を
先に行う）:

```json
"Stop": [
  { "hooks": [
      { "type": "command", "command": "~/.claude/hooks/lint-changed.sh" },
      { "type": "command", "command": "~/.claude/hooks/skill-observe-nudge.sh" },
      { "type": "command", "command": "~/.claude/hooks/notify-macos.sh" }
  ] }
]
```

### 3. 編集: `ai-agents/skills/skill-observe/SKILL.md`

- `## Notes` に追記: 「observation は Stop hook（`skill-observe-nudge.sh`）が**自動記録**する
  ようになった。本スキルは手動・補足記録およびバッチモード（引数なし）用として残る」。
- skill-authoring ルールに従い `metadata.version` を 1 → 2 にバンプ。

### 4. 編集: `.claude/rules/skill-authoring.md`（任意・軽微）

改善ループの記述に「Observe フェーズは Stop hook により自動記録される（手動は補足用）」の
一文を追加し、実態と整合させる。

## デプロイ

ソースは `ai-agents/settings/claude/` のみ変更。配布は既存ターゲットにそのまま乗る:

```sh
make claude-settings-copy   # 新 hook と settings.json を ~/.claude/ へ配布
```

（`make settings-copy` は cursor/copilot も含むが、本変更は claude 配下のみなので
`claude-settings-copy` で十分。）

## 検証

**静的チェック**:

- `shellcheck ai-agents/settings/claude/hooks/skill-observe-nudge.sh`
- `jq . ai-agents/settings/claude/settings.json`（JSON 妥当性）

**検知ロジックの単体確認**（実セッション前）:

- scratchpad に「リポジトリスキル（例 `/review`）を使った合成 transcript JSONL」を作り、
  `{"transcript_path":"...","session_id":"test","cwd":"<repo>"}` を stdin で渡して実行 →
  `decision:block` と `pending` に該当スキルが出ることを確認。
- `/schedule` など非リポジトリスキルだけの transcript では何も出力しない（`exit 0`）ことを確認。
- 同じ stdin を2回流し、2回目は状態ファイルにより無出力（再 nudge しない）ことを確認。

**E2E**（`make claude-settings-copy` 後、実セッション）:

1. `/review` 等のリポジトリスキルを使ってターンを終える → hook がブロックし、Claude が
   `ai-agents/skills/review/observations/<日付>_001_obs.md` を自動生成することを確認。
2. 生成ファイルが6フィールド形式かつ markdownlint を通ることを確認。
3. もう一度ターンを終える → 再ブロックされず通知のみ（ループしない）ことを確認。
4. `/skill-improve review` が新しい observation を読めることを確認。

## スコープ外

- `skill-improve`（Inspect/Amend）の自動化はしない（人間承認が必要な設計のため手動維持）。
- eval/benchmark 駆動の改善（公式 skill-creator 相当）は本 Issue では扱わない。
- issue #443 の `skill-scaffold`（Create フェーズ）は別タスク。
