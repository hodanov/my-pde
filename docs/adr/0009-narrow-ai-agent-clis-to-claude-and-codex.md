# ADR-0009: Narrow the supported AI agent CLIs to Claude Code and Codex CLI

- Status: Accepted (2026-10-10)

## Context

`ai-agents/` は Claude Code・Codex CLI・Cursor・Copilot の 4 CLI へ設定を配る構成だった。どのエージェントが主流になるか分からない時期に、いつでもピボットできるようにするための保険として整えたものだ。

今後は Claude Code と Codex CLI を主軸に使う方針にした。Cursor / Copilot の設定を維持する理由が薄れた一方で、保守コストは残っていた。

- `ai-agents/settings/cursor/` と `ai-agents/settings/copilot/` が、formatter 6 本・`guard-secret-read`・`get_file_path.py` を CLI ごとの自前コピーとして持つ（各 10 ファイル）。
- mise に cursor / copilot 専用のタスクが 10 本ある。
- 新規 hook のたびに「3 エディタ分の配線」を要求する前提が、`hook-scaffold`・`AGENTS.md`・2 本の routine プロンプトに埋まっている。
- scan routine がその前提から移植系の issue（#875: `guard-dangerous-bash` の cursor / copilot 移植）を出す。

両 CLI は hook の機能差も大きく、横展開は元々ファイル編集系に限っていた。cursor は一部イベントしか発火しない報告があり、copilot の `userPromptSubmitted` は `additionalContext` を無視する。claude 専用の hook（`lint-changed`・`guard-dangerous-bash` など）も多く、CLI によって遮断が効くかどうかが変わる状態だった。

## Options considered

| 案                                                                     | 退けた理由                                                                                                                                                      |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 現状維持（4 CLI 配布）                                                 | 主軸にしない 2 CLI のために、hook 追加のたびの配線・検証と routine プロンプトの前提を維持し続ける。#875 のように、移植だけで保守対象が増える issue が出続ける。 |
| 配布だけ止めて `settings/{cursor,copilot}` は repo に残す              | 配布も検証もされない設定は腐る。必要になった時点で再検証が要るなら、git 履歴から戻すのと手間が変わらない。                                                      |
| Cursor / Copilot だけ削除し、Codex は skills と `AGENTS.md` のみのまま | 主軸の Codex に実行前の遮断層が無い状態が残る。#875 の問題意識（CLI によって不可逆操作が通る）は移植先が Codex に変わるだけで消えない。                         |

## Decision

対象を Claude Code と Codex CLI に絞る。Cursor / Copilot 用の `ai-agents/settings/{cursor,copilot}/`、mise タスク、文書、routine プロンプトの前提を repo から削除する。

決め手は、主軸にしない CLI の設定は検証されず、残しても保険として機能しないこと。戻したくなったときのコストは git 履歴から戻す程度で、持ち続けるコストを下回る。

## Consequences

新規 hook の配線先は claude になる。Codex への hook 配線は、この変更では扱わず、別の変更で追加する。

`ai-agents/scripts/copy-entries.sh` は上書きコピーのみで削除を伝播しない（ADR-0007 と同じ）。`~/.cursor` と `~/.copilot` に配布済みのファイルは手動で消す。

次は意図して残した。

- `ai-agents/settings/claude/settings.json` の `Read(~/.cursor/**)` deny。`~/.cursor` の実体が残るため、安全側に倒す。
- `scripts/ai-bridge` の `AI_BRIDGE_CLI=cursor`。CLI 名を渡すだけの汎用指定で、設定配布とは無関係。
- `dotfiles/wezterm/ai-panes.lua` の `cursor-agent` / `copilot` の色分け。実行中プロセスの表示で、外しても得るものが小さい。

見直す条件は、Cursor / Copilot を再び主軸に使うようになったとき。その場合は、この変更の親コミットから `ai-agents/settings/{cursor,copilot}` と mise タスクを戻す。4 CLI 配布に戻すか、使う CLI だけ戻すかは新しい ADR で決め、本 ADR を Superseded にする。

関連: [#875](https://github.com/hodanov/my-pde/issues/875)（不採用で close）、ADR-0007
